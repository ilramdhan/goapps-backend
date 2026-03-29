// Package main is the entry point for the IAM service.
package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"

	iamv1 "github.com/mutugading/goapps-backend/gen/iam/v1"
	authapp "github.com/mutugading/goapps-backend/services/iam/internal/application/auth"
	grpcdelivery "github.com/mutugading/goapps-backend/services/iam/internal/delivery/grpc"
	httpdelivery "github.com/mutugading/goapps-backend/services/iam/internal/delivery/httpdelivery"
	"github.com/mutugading/goapps-backend/services/iam/internal/infrastructure/config"
	emailinfra "github.com/mutugading/goapps-backend/services/iam/internal/infrastructure/email"
	"github.com/mutugading/goapps-backend/services/iam/internal/infrastructure/jwt"
	"github.com/mutugading/goapps-backend/services/iam/internal/infrastructure/postgres"
	redisinfra "github.com/mutugading/goapps-backend/services/iam/internal/infrastructure/redis"
	storageinfra "github.com/mutugading/goapps-backend/services/iam/internal/infrastructure/storage"
	"github.com/mutugading/goapps-backend/services/iam/internal/infrastructure/totp"
	"github.com/mutugading/goapps-backend/services/iam/internal/infrastructure/tracing"
)

func main() {
	if err := run(); err != nil {
		log.Fatal().Err(err).Msg("Service failed")
	}
}

// run contains the main application logic, separated for cleaner error handling.
func run() error {
	setupLogger()

	// Load configuration
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	log.Info().
		Str("service", cfg.App.Name).
		Str("version", cfg.App.Version).
		Str("environment", cfg.App.Env).
		Msg("Starting IAM service")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Setup tracing (optional)
	cleanupTracing := setupTracing(ctx, cfg)
	defer cleanupTracing()

	// Setup database
	db, err := setupDatabase(cfg)
	if err != nil {
		return err
	}
	defer closeDatabase(db)

	// Setup Redis (optional - graceful degradation)
	redisClient, sessionCache, otpCache, rateLimitCache := setupRedis(cfg)
	if redisClient != nil {
		defer closeRedis(redisClient)
	}

	// Setup infrastructure services
	jwtService := jwt.NewService(&cfg.JWT)
	totpService := totp.NewService(&cfg.TOTP)

	// Setup repositories
	userRepo := postgres.NewUserRepository(db)
	sessionRepo := postgres.NewSessionRepository(db)
	roleRepo := postgres.NewRoleRepository(db)
	permRepo := postgres.NewPermissionRepository(db)
	userRoleRepo := postgres.NewUserRoleRepository(db)
	userPermissionRepo := postgres.NewUserPermissionRepository(db)
	auditRepo := postgres.NewAuditRepository(db)
	menuRepo := postgres.NewMenuRepository(db)
	companyRepo := postgres.NewCompanyRepository(db)
	divisionRepo := postgres.NewDivisionRepository(db)
	departmentRepo := postgres.NewDepartmentRepository(db)
	sectionRepo := postgres.NewSectionRepository(db)
	cmsPageRepo := postgres.NewCMSPageRepository(db)
	cmsSectionRepo := postgres.NewCMSSectionRepository(db)
	cmsSettingRepo := postgres.NewCMSSettingRepository(db)

	// Setup auth service
	authService := authapp.NewService(
		userRepo, sessionRepo, auditRepo,
		jwtService, totpService,
		sessionCache, otpCache, rateLimitCache,
		&cfg.Security,
	)

	// Setup email service
	emailService := emailinfra.NewService(&cfg.Email)
	authService.SetEmailService(emailService)

	// Setup validation helper
	validationHelper, err := grpcdelivery.NewValidationHelper()
	if err != nil {
		return err
	}

	// Setup storage service (optional - graceful degradation)
	var storageSvc storageinfra.Service
	minioSvc, err := storageinfra.NewMinIOService(storageinfra.Config{
		Endpoint:           cfg.Storage.Endpoint,
		AccessKey:          cfg.Storage.AccessKey,
		SecretKey:          cfg.Storage.SecretKey,
		Bucket:             cfg.Storage.Bucket,
		BasePath:           cfg.Storage.BasePath,
		UseSSL:             cfg.Storage.UseSSL,
		InsecureSkipVerify: cfg.Storage.InsecureSkipVerify,
		Region:             cfg.Storage.Region,
		PublicURL:          cfg.Storage.PublicURL,
	})
	if err != nil {
		log.Warn().Err(err).Msg("Failed to connect to MinIO storage, avatar uploads disabled")
		// storageSvc stays nil (interface nil, not concrete pointer nil)
	} else {
		storageSvc = minioSvc
	}

	// Setup gRPC handlers
	authHandler := grpcdelivery.NewAuthHandler(authService, userRepo, sessionRepo, auditRepo, validationHelper)
	userHandler := grpcdelivery.NewUserHandler(userRepo, userRoleRepo, userPermissionRepo, validationHelper, storageSvc)
	roleHandler := grpcdelivery.NewRoleHandler(roleRepo, validationHelper)
	permissionHandler := grpcdelivery.NewPermissionHandler(permRepo, validationHelper)
	sessionHandler := grpcdelivery.NewSessionHandler(sessionRepo, validationHelper)
	auditHandler := grpcdelivery.NewAuditHandler(auditRepo, validationHelper)
	menuHandler := grpcdelivery.NewMenuHandler(menuRepo, validationHelper)
	companyHandler := grpcdelivery.NewCompanyHandler(companyRepo, validationHelper)
	divisionHandler := grpcdelivery.NewDivisionHandler(divisionRepo, validationHelper)
	departmentHandler := grpcdelivery.NewDepartmentHandler(departmentRepo, validationHelper)
	sectionHandler := grpcdelivery.NewSectionHandler(sectionRepo, validationHelper)
	cmsPageHandler := grpcdelivery.NewCMSPageHandler(cmsPageRepo, validationHelper)
	cmsSectionHandler := grpcdelivery.NewCMSSectionHandler(cmsSectionRepo, cmsSettingRepo, storageSvc, validationHelper)
	cmsSettingHandler := grpcdelivery.NewCMSSettingHandler(cmsSettingRepo, validationHelper)

	// Setup gRPC server with interceptor chain (pass JWT + session cache + session repo for auth & activity tracking)
	grpcServer, err := grpcdelivery.NewServer(&cfg.Server, db, jwtService, sessionCache, sessionRepo)
	if err != nil {
		return err
	}

	// Register all IAM services
	gs := grpcServer.GRPCServer()
	iamv1.RegisterAuthServiceServer(gs, authHandler)
	iamv1.RegisterUserServiceServer(gs, userHandler)
	iamv1.RegisterRoleServiceServer(gs, roleHandler)
	iamv1.RegisterPermissionServiceServer(gs, permissionHandler)
	iamv1.RegisterSessionServiceServer(gs, sessionHandler)
	iamv1.RegisterAuditServiceServer(gs, auditHandler)
	iamv1.RegisterMenuServiceServer(gs, menuHandler)
	iamv1.RegisterCompanyServiceServer(gs, companyHandler)
	iamv1.RegisterDivisionServiceServer(gs, divisionHandler)
	iamv1.RegisterDepartmentServiceServer(gs, departmentHandler)
	iamv1.RegisterSectionServiceServer(gs, sectionHandler)
	iamv1.RegisterCMSPageServiceServer(gs, cmsPageHandler)
	iamv1.RegisterCMSSectionServiceServer(gs, cmsSectionHandler)
	iamv1.RegisterCMSSettingServiceServer(gs, cmsSettingHandler)

	// Start gRPC server
	go func() {
		if err := grpcServer.Start(); err != nil {
			log.Error().Err(err).Msg("gRPC server failed")
		}
	}()

	// Start HTTP gateway (Swagger, health, metrics, CORS)
	httpServer := httpdelivery.NewServer(&cfg.Server,
		httpdelivery.WithCORS(cfg.CORS.AllowedOrigins, cfg.CORS.MaxAge),
	)
	go func() {
		if err := httpServer.Start(ctx); err != nil {
			log.Warn().Err(err).Msg("HTTP server stopped")
		}
	}()

	// Wait for shutdown signal
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Info().Msg("Shutting down servers...")

	// Graceful shutdown
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()

	// Stop HTTP server
	if err := httpServer.Stop(shutdownCtx); err != nil {
		log.Error().Err(err).Msg("HTTP server shutdown error")
	}

	// Stop gRPC server
	grpcServer.Stop()

	log.Info().Msg("Server shutdown complete")
	return nil
}

// setupLogger configures the application logger.
func setupLogger() {
	zerolog.TimeFieldFormat = time.RFC3339
	if os.Getenv("APP_ENV") == "development" {
		log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stderr})
	}
}

// setupTracing initializes tracing and returns a cleanup function.
func setupTracing(ctx context.Context, cfg *config.Config) func() {
	tracingProvider, err := tracing.NewProvider(ctx, &cfg.Tracing, cfg.App.Name, cfg.App.Version)
	if err != nil {
		log.Warn().Err(err).Msg("Failed to setup tracing, continuing without it")
		return func() {}
	}

	if tracingProvider == nil {
		return func() {}
	}

	return func() {
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer shutdownCancel()
		if err := tracingProvider.Shutdown(shutdownCtx); err != nil {
			log.Warn().Err(err).Msg("Failed to shutdown tracing provider")
		}
	}
}

// setupDatabase creates a database connection.
func setupDatabase(cfg *config.Config) (*postgres.DB, error) {
	db, err := postgres.NewConnection(&cfg.Database)
	if err != nil {
		return nil, err
	}

	log.Info().
		Str("host", cfg.Database.Host).
		Int("port", cfg.Database.Port).
		Str("database", cfg.Database.Name).
		Msg("Database connection established")

	return db, nil
}

// closeDatabase closes the database connection.
func closeDatabase(db *postgres.DB) {
	if err := db.Close(); err != nil {
		log.Warn().Err(err).Msg("Failed to close database connection")
	}
}

// setupRedis creates a Redis connection (optional - graceful degradation).
func setupRedis(cfg *config.Config) (*redisinfra.Client, *redisinfra.SessionCache, *redisinfra.OTPCache, *redisinfra.RateLimitCache) {
	redisClient, err := redisinfra.NewClient(&cfg.Redis)
	if err != nil {
		log.Warn().Err(err).Msg("Failed to connect to Redis, continuing without cache")
		return nil, nil, nil, nil
	}

	sessionCache := redisinfra.NewSessionCache(redisClient, &cfg.Redis)
	otpCache := redisinfra.NewOTPCache(redisClient, cfg.Security.OTPExpiry)
	rateLimitCache := redisinfra.NewRateLimitCache(redisClient)

	log.Info().
		Str("host", cfg.Redis.Host).
		Int("port", cfg.Redis.Port).
		Msg("Redis connection established")

	return redisClient, sessionCache, otpCache, rateLimitCache
}

// closeRedis closes the Redis connection.
func closeRedis(client *redisinfra.Client) {
	if err := client.Close(); err != nil {
		log.Warn().Err(err).Msg("Failed to close Redis connection")
	}
}
