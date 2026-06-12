# Changelog

## [0.15.1](https://github.com/mutugading/goapps-backend/compare/iam-service/v0.15.0...iam-service/v0.15.1) (2026-06-12)


### Bug Fixes

* **iam:** fix email logo and social icons for Gmail/Outlook compatibility ([#118](https://github.com/mutugading/goapps-backend/issues/118)) ([1d9b0c5](https://github.com/mutugading/goapps-backend/commit/1d9b0c53c4db9f0bf905fb92299c0db0f59d7106))

## [0.15.0](https://github.com/mutugading/goapps-backend/compare/iam-service/v0.14.0...iam-service/v0.15.0) (2026-06-12)


### Features

* **bi-service:** Enhance BI module with migrations, dashboard features, and gRPC support ([060b249](https://github.com/mutugading/goapps-backend/commit/060b2495db306f617b04385f1f68bdbecc483f17))
* **bi:** add EBITDA + NET_PROFIT per-dashboard sidebar menu entries (IAM 000047) ([cc97e61](https://github.com/mutugading/goapps-backend/commit/cc97e6139a004e5ea28d9d7aa146bcd77e4a15c4))
* **bi:** seed DELIVERY_MARGIN dashboard config + IAM menu/permissions (000324+000046) ([ab4dcef](https://github.com/mutugading/goapps-backend/commit/ab4dcefdc7bdb9c569697aeb991d40eed8c99299))
* **costing:** Cost Fill Assignments, CPR Workflow, and IAM Notification Redesign ([5399842](https://github.com/mutugading/goapps-backend/commit/5399842feed2de8951891db737d72f877e13e3c0))
* **finance:** wire SLA + fill-assignment reminder notifications to cron scheduler ([e2d09d3](https://github.com/mutugading/goapps-backend/commit/e2d09d30c253885f87a2f9d7f319db6ee1e6d34b))
* **iam/db:** add migrations 000050-000053 for CPR roles, permissions, and section assignments ([de311c6](https://github.com/mutugading/goapps-backend/commit/de311c6ab31c34ae47b20b184fd2d4ad93354336))
* **iam/email:** add app_url config, CTA link routing, and asset embedding ([3aa4c78](https://github.com/mutugading/goapps-backend/commit/3aa4c780d032a13eeac6565cdf693ba94980b34d))
* **iam/email:** add AppName/AppURL/SupportURL to EmailConfig; fix from_name ([be9b8ab](https://github.com/mutugading/goapps-backend/commit/be9b8abeccb723ec6129b452b89d6fec7c92c97b))
* **iam/email:** add base shell and OTP HTML templates; OTP renderer test ([a183a5c](https://github.com/mutugading/goapps-backend/commit/a183a5c52625bfe5d1280c102819d3371139e2f5))
* **iam/email:** add multipart MIME builder with attachment support ([fecc2dd](https://github.com/mutugading/goapps-backend/commit/fecc2dd8dacdf4e02c2567de895ec400a1c529c2))
* **iam/email:** add notification template with table/alert/CTA support ([0a61538](https://github.com/mutugading/goapps-backend/commit/0a615388cdbb7896d9d39ec5375f170c4a7bf3dd))
* **iam/email:** add Renderer with data structs, template cache, SplitOTP/SplitParagraphs ([0264e60](https://github.com/mutugading/goapps-backend/commit/0264e60b94c17a3649105c5280a32be67e6cc98a))
* **iam/email:** add security and welcome HTML templates with integration tests ([1ae3da4](https://github.com/mutugading/goapps-backend/commit/1ae3da48afe3398ded9e0b509509ae5c03effed3))
* **iam/email:** redesign email templates — branding header, social icons, mobile responsive ([3f98295](https://github.com/mutugading/goapps-backend/commit/3f98295d1ebae2418b6dafe864afe2c3725b7627))
* **iam/email:** rewrite Service to use Renderer; add WithAttachments/WithTable methods; clean subjects ([86ef918](https://github.com/mutugading/goapps-backend/commit/86ef918f88862d16c51ae036b51f51cf0c2d7b18))
* **iam/email:** wire Renderer into main.go; email template system complete ([01e1a49](https://github.com/mutugading/goapps-backend/commit/01e1a4948c1986698fbad44582afee8af124cf8a))
* **iam:** add RequestHandler for rule-based notification fan-out with deduplication ([7b33a7d](https://github.com/mutugading/goapps-backend/commit/7b33a7d621793d865d172cf68fa3da24e54dbb8a))
* **iam:** add UserResolver interface and DB implementation for notification recipient resolution ([6fd761e](https://github.com/mutugading/goapps-backend/commit/6fd761e4ab673e253d58e028fc8e535286124a90))
* **iam:** add ValidateUnlockPassword use case ([c0b07d1](https://github.com/mutugading/goapps-backend/commit/c0b07d15bef450abd91034652a97d4787696fbd5))
* **iam:** cache user permissions in Redis on login/refresh; auth interceptor resolves from cache ([4789cd5](https://github.com/mutugading/goapps-backend/commit/4789cd5bd768895d96e48f22f36531e13fee07f8))
* **iam:** commit PRD v1.3 batch — company-mapping, approval workflow, encrypted chat ([ce4da89](https://github.com/mutugading/goapps-backend/commit/ce4da8924091c890406d5c9d0c0459ff17c9da2c))
* **iam:** implement NotificationEmailDispatcher; wire into RequestHandler so CPR events send emails via SMTP/Mailpit ([ee34d9f](https://github.com/mutugading/goapps-backend/commit/ee34d9f39973313944de6f3e9abb597b12455209))
* **iam:** implement RequestNotification gRPC handler with rule-based UserResolver and wire into server ([6080129](https://github.com/mutugading/goapps-backend/commit/60801293c86d999da5f3ee6258ba2b44f351f835))
* **iam:** populate section_code/department_code in AuthUser (Task 6 A3) ([baec8af](https://github.com/mutugading/goapps-backend/commit/baec8af8957ccc7ad70ebc20ff687b42117d0585))
* **iam:** seed BI permissions, menus, and SUPER_ADMIN role assignments ([23b7dda](https://github.com/mutugading/goapps-backend/commit/23b7ddae8b7f37d951e4e59eb141cc29c5b68b2d))
* **iam:** seed calc jobs + cost results sidebar menus ([6874c9b](https://github.com/mutugading/goapps-backend/commit/6874c9b6921d2869bb0d6cc4e9223b2c864c25b9))
* **iam:** seed fill-assignment permissions for finance costing ([b6cf87b](https://github.com/mutugading/goapps-backend/commit/b6cf87bac7df0552da14ea846e0c476666087903))
* **iam:** seed permissions for cost calc engine ([979590e](https://github.com/mutugading/goapps-backend/commit/979590eeef97cede35f86739e7433c1caba3b19e))
* **iam:** seed product costing permissions + sidebar menu (phase 1) ([2f9a284](https://github.com/mutugading/goapps-backend/commit/2f9a2845c9f774db90149e8eaca2520aec5ade5a))
* **product-cost:** Implement product costing system with migrations, handlers, and services ([173ddc6](https://github.com/mutugading/goapps-backend/commit/173ddc60c61e280da79ef78401ce0e73dd207a80))


### Bug Fixes

* **bi-dashboard:** Enhance BI metrics and dashboards with schema v1.1 updates ([bbb088d](https://github.com/mutugading/goapps-backend/commit/bbb088d8cd7c199f936113f11731506dd76a711c))
* **ci:** fix goimports local-prefix grouping across finance and iam ([7a73b25](https://github.com/mutugading/goapps-backend/commit/7a73b254c454ef94646514c95cba77822cec5d0e))
* **ci:** resolve golangci-lint and test failures on PR [#117](https://github.com/mutugading/goapps-backend/issues/117) ([0c9fd0f](https://github.com/mutugading/goapps-backend/commit/0c9fd0f8dca878a8f9e17c87b7948c7efef40e23))
* Dynamic YTD KPI for BI Dashboards & IAM Menu Permissions ([#115](https://github.com/mutugading/goapps-backend/issues/115)) ([69e9171](https://github.com/mutugading/goapps-backend/commit/69e9171597b06b47b512dd46b151ee1ed7cf51ab))
* **iam:** add audit fields + soft-delete guard to migration 000045 ([b098fa8](https://github.com/mutugading/goapps-backend/commit/b098fa8d338b0ac8ddf7507bc73294795b891feb))
* **iam:** add RequestNotification to permission interceptor allow-list (internal service RPC) ([40aea89](https://github.com/mutugading/goapps-backend/commit/40aea8919e88ac04ec894806f067256b97d36c6e))
* **iam:** company mapping delete only blocks on active users; return 409 on ErrAssignedToUser/ErrComboTaken ([079db53](https://github.com/mutugading/goapps-backend/commit/079db530d7b594c12e38e5ec4dc7b819a68d9fc0))
* **iam:** correct FINANCE_PRODUCT_ORDERS menu url to /finance/routes ([7a1c989](https://github.com/mutugading/goapps-backend/commit/7a1c989407e258725634f1d713a34fb4b1ec0f8d))
* **iam:** correct FINANCE_PRODUCT_ORDERS menu url to /finance/routes ([0bc4606](https://github.com/mutugading/goapps-backend/commit/0bc4606f7930093d317ab3b31d6db5dae0155374))
* **iam:** gate RM Pricing and Product Costing parent menus ([082fd67](https://github.com/mutugading/goapps-backend/commit/082fd6721bf75e067d8c170369a92cbbaabd39e7))
* **iam:** remove finance.product.request.create from CPR_ADMIN role ([4c401b4](https://github.com/mutugading/goapps-backend/commit/4c401b4145b8bf9fe426fdb7800b6d738b9feff8))
* **iam:** replace interface{} with any in UserResolver scan helper ([6066175](https://github.com/mutugading/goapps-backend/commit/606617547058c72e4acb952065ff35000263ba13))
* **iam:** resolve golangci-lint v2.3.0 CI failures (74 issues → 0) ([b33f749](https://github.com/mutugading/goapps-backend/commit/b33f7498af9fe51498e22d7d06a15654a6bfa82d))

## [0.14.0](https://github.com/mutugading/goapps-backend/compare/iam-service/v0.13.0...iam-service/v0.14.0) (2026-05-07)


### Features

* **auth:** support internal service token bypass for trusted backend callers ([24aaafe](https://github.com/mutugading/goapps-backend/commit/24aaafe3e99836ae0e9ebf1afacc4af97147e511))
* **export-notification:** Implement generic IAM notification system with MinIO export support ([124f619](https://github.com/mutugading/goapps-backend/commit/124f619c16e4f2ef5bc70c975fc9e40deb2e1244))
* **notification:** add generic IAM notification system with SSE realtime delivery ([7682d14](https://github.com/mutugading/goapps-backend/commit/7682d1499e85285f728f9c7dd8cfa18725a2724f))


### Bug Fixes

* **lint:** resolve golangci-lint errors and apply Copilot review feedback ([9b96fd5](https://github.com/mutugading/goapps-backend/commit/9b96fd5fc97b200577cca144b5d3656a6930f639))
* **lint:** resolve remaining gocyclo, gocognit, and errorlint failures ([962dcfd](https://github.com/mutugading/goapps-backend/commit/962dcfdad6700bb843643589396696dc7b4d4221))
* **notification:** suppress nilerr on intentional bad-cursor swallow ([bcc6a50](https://github.com/mutugading/goapps-backend/commit/bcc6a5024e2322fd58835d3c77e2ff625461933f))
* **tracing:** fetch tracer lazily per-request to survive late provider init ([0186299](https://github.com/mutugading/goapps-backend/commit/018629984683afb9ba47c9f02ee198a36f835336))
* **tracing:** fetch tracer lazily per-request to survive late provider init ([788d0bc](https://github.com/mutugading/goapps-backend/commit/788d0bc6c0db634bd90430354542d21086b541a5))
* **tracing:** use otlptracegrpc.WithInsecure() to actually disable TLS ([0908955](https://github.com/mutugading/goapps-backend/commit/090895526959ecbc8deaa37a8f7fc1dc661aa8b7))

## [0.13.0](https://github.com/mutugading/goapps-backend/compare/iam-service/v0.12.0...iam-service/v0.13.0) (2026-04-22)


### Features

* implement raw material grouping and cost management modules with associated gRPC services and database migrations ([f67d111](https://github.com/mutugading/goapps-backend/commit/f67d111cb998323e80f8d3a8b9b93859227af4fa))
* implement raw material grouping and cost management modules with associated gRPC services and database migrations ([a24776a](https://github.com/mutugading/goapps-backend/commit/a24776a45003a72248a9c45c0d35dd776d23ada8))

## [0.12.0](https://github.com/mutugading/goapps-backend/compare/iam-service/v0.11.0...iam-service/v0.12.0) (2026-04-17)


### Features

* **finance:** implement job execution tracking, Oracle sync, and RabbitMQ integration with migrations and handlers ([8f9896c](https://github.com/mutugading/goapps-backend/commit/8f9896c2de11dbb5428a71cb20430bec773a7a05))
* **finance:** implement job execution tracking, Oracle sync, and RabbitMQ integration with migrations and handlers ([02db90f](https://github.com/mutugading/goapps-backend/commit/02db90f9eee87a72611700192d10614266cd65b8))


### Bug Fixes

* **finance:** enhance Oracle sync system with improved error handling, concurrency safety, and refined data validation ([4ce67ee](https://github.com/mutugading/goapps-backend/commit/4ce67eeb620dd423b9dab6a77bfe52ad3c21c839))

## [0.11.0](https://github.com/mutugading/goapps-backend/compare/iam-service/v0.10.3...iam-service/v0.11.0) (2026-04-16)


### Features

* **iam:** add Employee Group management module with migrations, handlers, gRPC, and protobuf changes ([00581ce](https://github.com/mutugading/goapps-backend/commit/00581ceedda8f7787ee41977b68b96ddb0f5ebb5))
* **iam:** add Employee Group management module with migrations, handlers, gRPC, and protobuf changes ([18e9d06](https://github.com/mutugading/goapps-backend/commit/18e9d06edd8eeaa49f54186cdab3e8a5985b3e80))

## [0.10.3](https://github.com/mutugading/goapps-backend/compare/iam-service/v0.10.2...iam-service/v0.10.3) (2026-04-16)


### Bug Fixes

* **iam:** exclude permissions from JWT to prevent oversized cookie ([fd9cbce](https://github.com/mutugading/goapps-backend/commit/fd9cbceba24be76dffffac54dfd46ec5d26e0a26))
* **iam:** exclude permissions from JWT to prevent oversized cookie ([d7c3f81](https://github.com/mutugading/goapps-backend/commit/d7c3f813972f5da93c768e6d16936188595dcb6c))

## [0.10.2](https://github.com/mutugading/goapps-backend/compare/iam-service/v0.10.1...iam-service/v0.10.2) (2026-04-15)


### Bug Fixes

* **iam:** include role-based permissions in GetRolesAndPermissions query ([1b659bb](https://github.com/mutugading/goapps-backend/commit/1b659bb9c23e77f0c66ced6dabdb5aae9a5ac573))
* **iam:** include role-based permissions in GetRolesAndPermissions query ([0d457ef](https://github.com/mutugading/goapps-backend/commit/0d457efd27baa383108ffeea30467a0579cbce55))

## [0.10.1](https://github.com/mutugading/goapps-backend/compare/iam-service/v0.10.0...iam-service/v0.10.1) (2026-04-15)


### Bug Fixes

* **chore:** add shared module copy step to Dockerfile iam and finance svc for dependency resolution ([ea159be](https://github.com/mutugading/goapps-backend/commit/ea159bee99c871929bc6dda7fc060a23c5431843))

## [0.10.0](https://github.com/mutugading/goapps-backend/compare/iam-service/v0.9.0...iam-service/v0.10.0) (2026-04-15)


### Features

* implement employee level management module and fix admin email verification migration ([45cff67](https://github.com/mutugading/goapps-backend/commit/45cff67eb0cbc83cbc910b912c2437ae62cfd6eb))
* implement employee level management module and fix admin email verification migration ([92428c6](https://github.com/mutugading/goapps-backend/commit/92428c61869c42b1333f3aa9ac440f4891d63f4b))


### Bug Fixes

* improve code safety and maintainability by implementing safe integer conversions, extracting entity validation methods, and enforcing linting standards. ([bf248da](https://github.com/mutugading/goapps-backend/commit/bf248dad4e2f6c56e130ed8fba454aaaee0092ec))

## [0.9.0](https://github.com/mutugading/goapps-backend/compare/iam-service/v0.8.2...iam-service/v0.9.0) (2026-04-14)


### Features

* Add build and copy steps for the `iam-seed` binary. ([b55b3f8](https://github.com/mutugading/goapps-backend/commit/b55b3f89c6b4a66511a944ae7ad1ec9933793110))
* Add golang-migrate and migration files to the IAM service Dockerfile for database migrations. ([1de4a14](https://github.com/mutugading/goapps-backend/commit/1de4a147c48dc90e56e40c0fd0e1ee4f00834e05))
* Add MinIO storage integration and user profile picture upload functionality. ([2754734](https://github.com/mutugading/goapps-backend/commit/27547346f3a6096fe3d4672a8fa7508f9835f2c3))
* Add MinIO storage integration and user profile picture upload functionality. ([8c8d7c0](https://github.com/mutugading/goapps-backend/commit/8c8d7c042e497002579b078fb57c6d2fb3ef9ff4))
* add UOM category management with CRUD, import/export, and seed migration ([29e4c43](https://github.com/mutugading/goapps-backend/commit/29e4c432fdaf2ac13f81dd548488ed6d5596c75f))
* add UOM category management with CRUD, import/export, and seed migration ([f4cbaaa](https://github.com/mutugading/goapps-backend/commit/f4cbaaaed866fec031f8a0af05bdb98970d99254))
* Implement 2FA recovery codes in IAM, add gRPC authentication and permission interceptors to IAM and Finance, and update local development infrastructure. ([c82c258](https://github.com/mutugading/goapps-backend/commit/c82c258bd39279f31789b9635367b2e4cb34d19b))
* Implement 2FA recovery codes in IAM, add gRPC authentication and permission interceptors to IAM and Finance, and update local development infrastructure. ([a1ad398](https://github.com/mutugading/goapps-backend/commit/a1ad3980e6594d80e4154b0ecf4dfcb8f42f09c1))
* implement CMS module with CRUD operations, database schema, and gRPC gateway support ([5eb8515](https://github.com/mutugading/goapps-backend/commit/5eb8515c646e89a934bbd5694c4b9833c6accad8))
* implement CMS module with CRUD operations, database schema, and gRPC gateway support ([c0df3b4](https://github.com/mutugading/goapps-backend/commit/c0df3b47eb36a97bf1eec792ac114ffa4d9571a8))
* implement finance parameter management module with CRUD, import/export, and UI components ([9505fd1](https://github.com/mutugading/goapps-backend/commit/9505fd10daa3aedf88a101e1aea7d0dfe572bcd3))
* implement formula management service with CRUD operations, gRPC definitions, and database schema ([ee9831c](https://github.com/mutugading/goapps-backend/commit/ee9831c156013d0b81b3634645934392dffef324))
* implement formula management service with CRUD operations, gRPC definitions, and database schema ([1faa10c](https://github.com/mutugading/goapps-backend/commit/1faa10ce43da4772131957ef9f53ce95987e5038))
* Implement IAM audit log APIs, enhance Swagger documentation with security and persistent authorization, and add a swagger merge script. ([55c70c8](https://github.com/mutugading/goapps-backend/commit/55c70c82a410ec6dbc328a1f0c1bec7609c6b84d))
* Implement session idle timeout, add comprehensive E2E tests, and include a backend run guide. ([5610f2c](https://github.com/mutugading/goapps-backend/commit/5610f2cc28d10f2c2a52db56fb24f539c98962a5))
* Implement the initial Identity and Access Management (IAM) serv… ([f950837](https://github.com/mutugading/goapps-backend/commit/f9508371b9f2a9106e8b6803ec8200affa44737f))
* Implement the initial Identity and Access Management (IAM) service with migrations, application logic, infrastructure, and gRPC delivery. ([96fe69f](https://github.com/mutugading/goapps-backend/commit/96fe69f34b0545fc6bd07b337e36ddf61812ee5d))


### Bug Fixes

* **email:** add SMTP client timeouts to prevent indefinite hanging ([020a81a](https://github.com/mutugading/goapps-backend/commit/020a81ae0a25ffca4b307d1ef4e41fa6a93e39c7))
* **email:** add SMTP client timeouts to prevent indefinite hanging ([331f073](https://github.com/mutugading/goapps-backend/commit/331f073c93f0fd9cf3632dcc2b6d750a81ebad25))
* **email:** handle errors when closing SMTP connection after SetDeadline failure ([9e1b2c3](https://github.com/mutugading/goapps-backend/commit/9e1b2c3b74ddeeafd904eb76fb14d59dc9f24325))
* **iam:** consolidate error mapping logic into mapUnknownError helper and include ErrEmailAlreadyVerified in conflict checks ([2f7d0ab](https://github.com/mutugading/goapps-backend/commit/2f7d0abf2263fc45863cc5a3a294c7e12f29a8b3))
* **iam:** implement email verification flow including database schema, cache storage, and gRPC service methods ([084bcac](https://github.com/mutugading/goapps-backend/commit/084bcac2d1f86c8cc28215d5ee503874a258ff0e))
* **iam:** implement email verification flow including database schema, cache storage, and gRPC service methods ([b1b0b3d](https://github.com/mutugading/goapps-backend/commit/b1b0b3d785b56b3ebc0c69c051dca02fba8bddfd))
* Implement IAM audit log APIs, enhance Swagger documentation with security and persistent authorization, and add a swagger merge script. ([3387841](https://github.com/mutugading/goapps-backend/commit/3387841280e082eded628ce235998d5e4721eccf))
* lint check. ([84ef101](https://github.com/mutugading/goapps-backend/commit/84ef10161e9a600e55b3679c6c82cce33003eb2c))
* Seed raw material categories with IAM menu and permissions, and refactor sort order strings to constants. ([1330c63](https://github.com/mutugading/goapps-backend/commit/1330c630b279efc4302ca31758489ba44f712ad3))

## [0.8.2](https://github.com/mutugading/goapps-backend/compare/iam-service/v0.8.1...iam-service/v0.8.2) (2026-04-14)


### Bug Fixes

* **iam:** consolidate error mapping logic into mapUnknownError helper and include ErrEmailAlreadyVerified in conflict checks ([2f7d0ab](https://github.com/mutugading/goapps-backend/commit/2f7d0abf2263fc45863cc5a3a294c7e12f29a8b3))
* **iam:** implement email verification flow including database schema, cache storage, and gRPC service methods ([084bcac](https://github.com/mutugading/goapps-backend/commit/084bcac2d1f86c8cc28215d5ee503874a258ff0e))
* **iam:** implement email verification flow including database schema, cache storage, and gRPC service methods ([b1b0b3d](https://github.com/mutugading/goapps-backend/commit/b1b0b3d785b56b3ebc0c69c051dca02fba8bddfd))

## [0.8.1](https://github.com/mutugading/goapps-backend/compare/iam-service/v0.8.0...iam-service/v0.8.1) (2026-04-14)


### Bug Fixes

* **email:** add SMTP client timeouts to prevent indefinite hanging ([020a81a](https://github.com/mutugading/goapps-backend/commit/020a81ae0a25ffca4b307d1ef4e41fa6a93e39c7))
* **email:** add SMTP client timeouts to prevent indefinite hanging ([331f073](https://github.com/mutugading/goapps-backend/commit/331f073c93f0fd9cf3632dcc2b6d750a81ebad25))
* **email:** handle errors when closing SMTP connection after SetDeadline failure ([9e1b2c3](https://github.com/mutugading/goapps-backend/commit/9e1b2c3b74ddeeafd904eb76fb14d59dc9f24325))

## [0.8.0](https://github.com/mutugading/goapps-backend/compare/iam-service/v0.7.0...iam-service/v0.8.0) (2026-04-13)


### Features

* add UOM category management with CRUD, import/export, and seed migration ([29e4c43](https://github.com/mutugading/goapps-backend/commit/29e4c432fdaf2ac13f81dd548488ed6d5596c75f))
* add UOM category management with CRUD, import/export, and seed migration ([f4cbaaa](https://github.com/mutugading/goapps-backend/commit/f4cbaaaed866fec031f8a0af05bdb98970d99254))

## [0.7.0](https://github.com/mutugading/goapps-backend/compare/iam-service/v0.6.0...iam-service/v0.7.0) (2026-04-08)


### Features

* implement formula management service with CRUD operations, gRPC definitions, and database schema ([ee9831c](https://github.com/mutugading/goapps-backend/commit/ee9831c156013d0b81b3634645934392dffef324))
* implement formula management service with CRUD operations, gRPC definitions, and database schema ([1faa10c](https://github.com/mutugading/goapps-backend/commit/1faa10ce43da4772131957ef9f53ce95987e5038))

## [0.6.0](https://github.com/mutugading/goapps-backend/compare/iam-service/v0.5.0...iam-service/v0.6.0) (2026-04-07)


### Features

* Add build and copy steps for the `iam-seed` binary. ([b55b3f8](https://github.com/mutugading/goapps-backend/commit/b55b3f89c6b4a66511a944ae7ad1ec9933793110))
* Add golang-migrate and migration files to the IAM service Dockerfile for database migrations. ([1de4a14](https://github.com/mutugading/goapps-backend/commit/1de4a147c48dc90e56e40c0fd0e1ee4f00834e05))
* implement CMS module with CRUD operations, database schema, and gRPC gateway support ([5eb8515](https://github.com/mutugading/goapps-backend/commit/5eb8515c646e89a934bbd5694c4b9833c6accad8))
* implement CMS module with CRUD operations, database schema, and gRPC gateway support ([c0df3b4](https://github.com/mutugading/goapps-backend/commit/c0df3b47eb36a97bf1eec792ac114ffa4d9571a8))
* implement finance parameter management module with CRUD, import/export, and UI components ([9505fd1](https://github.com/mutugading/goapps-backend/commit/9505fd10daa3aedf88a101e1aea7d0dfe572bcd3))


### Bug Fixes

* lint check. ([84ef101](https://github.com/mutugading/goapps-backend/commit/84ef10161e9a600e55b3679c6c82cce33003eb2c))
* Seed raw material categories with IAM menu and permissions, and refactor sort order strings to constants. ([1330c63](https://github.com/mutugading/goapps-backend/commit/1330c630b279efc4302ca31758489ba44f712ad3))

## [0.5.0](https://github.com/mutugading/goapps-backend/compare/iam-service/v0.4.0...iam-service/v0.5.0) (2026-03-20)


### Features

* Add MinIO storage integration and user profile picture upload functionality. ([2754734](https://github.com/mutugading/goapps-backend/commit/27547346f3a6096fe3d4672a8fa7508f9835f2c3))
* Add MinIO storage integration and user profile picture upload functionality. ([8c8d7c0](https://github.com/mutugading/goapps-backend/commit/8c8d7c042e497002579b078fb57c6d2fb3ef9ff4))
* Implement session idle timeout, add comprehensive E2E tests, and include a backend run guide. ([5610f2c](https://github.com/mutugading/goapps-backend/commit/5610f2cc28d10f2c2a52db56fb24f539c98962a5))

## [0.4.0](https://github.com/mutugading/goapps-backend/compare/iam-service/v0.3.0...iam-service/v0.4.0) (2026-02-09)


### Features

* Implement IAM audit log APIs, enhance Swagger documentation with security and persistent authorization, and add a swagger merge script. ([55c70c8](https://github.com/mutugading/goapps-backend/commit/55c70c82a410ec6dbc328a1f0c1bec7609c6b84d))


### Bug Fixes

* Implement IAM audit log APIs, enhance Swagger documentation with security and persistent authorization, and add a swagger merge script. ([3387841](https://github.com/mutugading/goapps-backend/commit/3387841280e082eded628ce235998d5e4721eccf))

## [0.3.0](https://github.com/mutugading/goapps-backend/compare/iam-service/v0.2.0...iam-service/v0.3.0) (2026-02-08)


### Features

* Implement 2FA recovery codes in IAM, add gRPC authentication and permission interceptors to IAM and Finance, and update local development infrastructure. ([c82c258](https://github.com/mutugading/goapps-backend/commit/c82c258bd39279f31789b9635367b2e4cb34d19b))
* Implement 2FA recovery codes in IAM, add gRPC authentication and permission interceptors to IAM and Finance, and update local development infrastructure. ([a1ad398](https://github.com/mutugading/goapps-backend/commit/a1ad3980e6594d80e4154b0ecf4dfcb8f42f09c1))

## [0.2.0](https://github.com/mutugading/goapps-backend/compare/iam-service/v0.1.0...iam-service/v0.2.0) (2026-02-07)


### Features

* Implement the initial Identity and Access Management (IAM) serv… ([f950837](https://github.com/mutugading/goapps-backend/commit/f9508371b9f2a9106e8b6803ec8200affa44737f))
* Implement the initial Identity and Access Management (IAM) service with migrations, application logic, infrastructure, and gRPC delivery. ([96fe69f](https://github.com/mutugading/goapps-backend/commit/96fe69f34b0545fc6bd07b337e36ddf61812ee5d))
