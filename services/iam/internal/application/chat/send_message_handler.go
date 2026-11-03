package chat

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"github.com/rs/zerolog/log"

	iamv1 "github.com/mutugading/goapps-backend/gen/iam/v1"
	appnotif "github.com/mutugading/goapps-backend/services/iam/internal/application/notification"
	"github.com/mutugading/goapps-backend/services/iam/internal/domain/chat"
	"github.com/mutugading/goapps-backend/services/iam/internal/domain/notification"
	chatinfra "github.com/mutugading/goapps-backend/services/iam/internal/infrastructure/chat"
	"github.com/mutugading/goapps-backend/services/iam/internal/infrastructure/crypto"
	"github.com/mutugading/goapps-backend/services/iam/internal/infrastructure/postgres"
)

// SendMessageHandler sends a new message to a conversation.
type SendMessageHandler struct {
	convRepo      chat.ConversationRepository
	msgRepo       chat.MessageRepository
	receiptRepo   chat.ReadReceiptRepository
	enc           *crypto.Encryptor
	broadcaster   *chatinfra.Broadcaster
	presence      *chatinfra.PresenceService
	notifCreate   *appnotif.CreateHandler
	emailDispatch appnotif.EmailDispatcher
	rdb           *redis.Client
	userResolver  *postgres.ChatUserResolver
	attRepo       chat.AttachmentRepository
}

// NewSendMessageHandler constructs the handler.
func NewSendMessageHandler(
	convRepo chat.ConversationRepository,
	msgRepo chat.MessageRepository,
	receiptRepo chat.ReadReceiptRepository,
	enc *crypto.Encryptor,
	broadcaster *chatinfra.Broadcaster,
) *SendMessageHandler {
	return &SendMessageHandler{convRepo: convRepo, msgRepo: msgRepo, receiptRepo: receiptRepo, enc: enc, broadcaster: broadcaster}
}

// WithAttachments enables linking uploaded attachments to sent messages.
func (h *SendMessageHandler) WithAttachments(attRepo chat.AttachmentRepository) *SendMessageHandler {
	h.attRepo = attRepo
	return h
}

// WithOfflineNotification enables email notifications for offline users.
func (h *SendMessageHandler) WithOfflineNotification(presence *chatinfra.PresenceService, notifCreate *appnotif.CreateHandler, emailDispatch appnotif.EmailDispatcher, rdb *redis.Client, userResolver *postgres.ChatUserResolver) *SendMessageHandler {
	h.presence = presence
	h.notifCreate = notifCreate
	h.emailDispatch = emailDispatch
	h.rdb = rdb
	h.userResolver = userResolver
	return h
}

// Handle validates participation, encrypts body, saves, and broadcasts. Any
// attachmentIDs that belong to this conversation and are not yet linked are
// attached to the new message. Returns the message plus its attachment protos.
func (h *SendMessageHandler) Handle(ctx context.Context, senderID, convID uuid.UUID, body string, replyToID uuid.UUID, attachmentIDs []uuid.UUID) (*chat.Message, []*iamv1.AttachmentProto, error) {
	conv, err := h.convRepo.GetByID(ctx, convID)
	if err != nil {
		return nil, nil, err
	}
	p := conv.FindParticipant(senderID)
	if p == nil || !p.IsActive() {
		return nil, nil, fmt.Errorf("send message: %w", chat.ErrNotParticipant)
	}

	convKeyPlain, err := h.enc.DecryptConversationKey(conv.EncryptionKey())
	if err != nil {
		return nil, nil, fmt.Errorf("send message: decrypt conv key: %w", err)
	}

	bodyEnc, err := h.enc.EncryptMessage(convKeyPlain, body)
	if err != nil {
		return nil, nil, fmt.Errorf("send message: encrypt body: %w", err)
	}
	bodyPlainEnc, err := h.enc.EncryptMessage(convKeyPlain, body) // same body, independent nonce for search index slot
	if err != nil {
		return nil, nil, fmt.Errorf("send message: encrypt plain: %w", err)
	}

	msg := chat.NewMessage(convID, senderID, bodyEnc, bodyPlainEnc, replyToID)
	if err := h.msgRepo.Create(ctx, msg); err != nil {
		return nil, nil, fmt.Errorf("send message: persist: %w", err)
	}

	// Auto-read for sender.
	if err := h.receiptRepo.Upsert(ctx, msg.MessageID(), senderID); err != nil {
		log.Warn().Err(err).Msg("send message: auto read receipt failed")
	}

	attProtos, err := h.linkAttachments(ctx, convID, msg.MessageID(), attachmentIDs)
	if err != nil {
		return nil, nil, err
	}

	senderName := resolveSenderName(ctx, h.userResolver, senderID)
	broadcastMessageEvent(h.broadcaster, conv, msg, body, "message_received", senderName, attProtos)

	h.notifyOfflineParticipants(ctx, conv, msg, senderID, senderName, body)
	return msg, attProtos, nil
}

// linkAttachments links pending attachments (belonging to convID and not yet
// linked) to messageID and returns their proto representations.
func (h *SendMessageHandler) linkAttachments(ctx context.Context, convID, messageID uuid.UUID, attachmentIDs []uuid.UUID) ([]*iamv1.AttachmentProto, error) {
	if len(attachmentIDs) == 0 || h.attRepo == nil {
		return nil, nil
	}
	found, err := h.attRepo.GetByIDs(ctx, attachmentIDs)
	if err != nil {
		return nil, fmt.Errorf("send message: load attachments: %w", err)
	}
	validIDs := make([]uuid.UUID, 0, len(found))
	protos := make([]*iamv1.AttachmentProto, 0, len(found))
	for _, att := range found {
		if att.ConversationID() != convID || att.MessageID() != nil {
			continue
		}
		validIDs = append(validIDs, att.AttachmentID())
		protos = append(protos, attachmentToProto(att))
	}
	if len(validIDs) == 0 {
		return nil, nil
	}
	if err := h.attRepo.LinkToMessage(ctx, messageID, validIDs); err != nil {
		return nil, fmt.Errorf("send message: link attachments: %w", err)
	}
	return protos, nil
}

const emailDebounceTTL = 5 * time.Minute

func (h *SendMessageHandler) notifyOfflineParticipants(ctx context.Context, conv *chat.Conversation, msg *chat.Message, senderID uuid.UUID, senderName, body string) {
	if h.presence == nil || h.notifCreate == nil {
		return
	}
	truncatedBody := body
	if len(truncatedBody) > 100 {
		truncatedBody = truncatedBody[:100] + "..."
	}
	for _, p := range conv.Participants() {
		if !p.IsActive() || p.UserID() == senderID {
			continue
		}
		h.notifyIfOffline(ctx, p.UserID(), msg, senderID, senderName, truncatedBody)
	}
}

func (h *SendMessageHandler) notifyIfOffline(ctx context.Context, recipientID uuid.UUID, msg *chat.Message, senderID uuid.UUID, senderName, truncatedBody string) {
	online, err := h.presence.IsOnline(ctx, recipientID)
	if err != nil {
		log.Warn().Err(err).Str("user", recipientID.String()).Msg("send message: check online status")
		return
	}
	if online {
		return
	}
	if h.isDebounced(ctx, msg.ConversationID(), recipientID) {
		return
	}
	title := "New chat message"
	body := truncatedBody
	if senderName != "" {
		title = fmt.Sprintf("%s sent you a message", senderName)
		body = fmt.Sprintf("%s: %s", senderName, truncatedBody)
	}
	n, err := h.notifCreate.Handle(ctx, appnotif.CreateCommand{
		RecipientUserID: recipientID,
		Type:            notification.TypeChat,
		Severity:        notification.SeverityInfo,
		Title:           title,
		Body:            body,
		ActionType:      notification.ActionNavigate,
		ActionPayload:   `{"path":"/chat"}`,
		SourceType:      "chat_message",
		SourceID:        msg.MessageID().String(),
		CreatedBy:       senderID.String(),
	})
	if err != nil {
		log.Warn().Err(err).Msg("send message: create offline notification")
		return
	}
	if h.emailDispatch != nil && n != nil {
		go h.emailDispatch.Dispatch(context.Background(), n) //nolint:gosec // G118: intentionally detached — email must not be cancelled with the request
	}
}

func (h *SendMessageHandler) isDebounced(ctx context.Context, convID, userID uuid.UUID) bool {
	if h.rdb == nil {
		return false
	}
	key := fmt.Sprintf("chat:email-debounce:%s:%s", convID, userID)
	set, err := h.rdb.SetNX(ctx, key, "1", emailDebounceTTL).Result()
	if err != nil {
		log.Warn().Err(err).Msg("send message: debounce check")
		return false
	}
	return !set
}
