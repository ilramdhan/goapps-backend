// Package prdrequest holds application-layer command handlers for the ProductRequest aggregate.
//
// Handlers coordinate domain logic (in internal/domain/prdrequest), persistence
// (via prdrequest.Repository), atomic ticket-no allocation (via prdrequest.TicketNoGenerator),
// and one cross-aggregate read for "match existing product" (via product.Repository.SearchByText).
//
// Phase 1 scope: ticket CRUD + state transitions (Assign, Resolve, Reject) +
// SearchExistingProducts (delegates to product domain). Notification emission, attachments,
// and comments land in later phases.
package prdrequest
