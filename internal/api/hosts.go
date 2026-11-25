package api

import (
	"context"
	"errors"

	"github.com/mitto98/homelab-proxy-manager/internal/model"
	"github.com/mitto98/homelab-proxy-manager/internal/store"
)

func toHost(h model.Host) Host {
	return Host{
		Id:                    h.ID,
		Kind:                  HostKind(h.Kind),
		Enabled:               h.Enabled,
		Domains:               h.Domains,
		UpstreamScheme:        HostUpstreamScheme(h.UpstreamScheme),
		UpstreamHost:          h.UpstreamHost,
		UpstreamPort:          h.UpstreamPort,
		UpstreamSkipTlsVerify: h.UpstreamSkipTLSVerify,
		CreatedAt:             h.CreatedAt,
		UpdatedAt:             h.UpdatedAt,
	}
}

// hostFromInput builds a normalized, validated Proxy Host from a request body.
// Optional booleans default to enabled = true and skip-TLS-verify = false.
func hostFromInput(id int64, in HostInput) (model.Host, error) {
	h := model.Host{
		ID:                    id,
		Kind:                  model.KindProxy,
		Enabled:               true,
		Domains:               append([]string(nil), in.Domains...),
		UpstreamScheme:        string(in.UpstreamScheme),
		UpstreamHost:          in.UpstreamHost,
		UpstreamPort:          in.UpstreamPort,
		UpstreamSkipTLSVerify: in.UpstreamSkipTlsVerify != nil && *in.UpstreamSkipTlsVerify,
	}
	if in.Enabled != nil {
		h.Enabled = *in.Enabled
	}
	h.Normalize()
	return h, h.Validate()
}

// invalidBody is the error body for a validation or conflict error: its message
// and, when known, the request field it is about.
func invalidBody(err error) Error {
	var ve *model.ValidationError
	if errors.As(err, &ve) {
		return Error{Error: ve.Message, Field: &ve.Field}
	}
	var taken *store.DomainTakenError
	if errors.As(err, &taken) {
		field := "domains"
		return Error{Error: taken.Error(), Field: &field}
	}
	return Error{Error: err.Error()}
}

var hostNotFound = Error{Error: "host not found"}

func (s *Server) ListHosts(ctx context.Context, _ ListHostsRequestObject) (ListHostsResponseObject, error) {
	hosts, err := s.store.ListHosts(ctx)
	if err != nil {
		return nil, err
	}
	out := make(ListHosts200JSONResponse, 0, len(hosts))
	for _, h := range hosts {
		out = append(out, toHost(h))
	}
	return out, nil
}

func (s *Server) CreateHost(ctx context.Context, req CreateHostRequestObject) (CreateHostResponseObject, error) {
	h, err := hostFromInput(0, *req.Body)
	if err != nil {
		return CreateHost400JSONResponse{ErrorJSONResponse(invalidBody(err))}, nil
	}
	var created model.Host
	_, err = s.applier.Apply(ctx, func(tx *store.Tx) (e error) {
		created, e = tx.CreateHost(ctx, h)
		return e
	})
	var taken *store.DomainTakenError
	if errors.As(err, &taken) {
		return CreateHost409JSONResponse(invalidBody(err)), nil
	}
	if err != nil {
		return nil, err
	}
	return CreateHost201JSONResponse(toHost(created)), nil
}

func (s *Server) GetHost(ctx context.Context, req GetHostRequestObject) (GetHostResponseObject, error) {
	h, err := s.store.GetHost(ctx, req.Id)
	if errors.Is(err, store.ErrNotFound) {
		return GetHost404JSONResponse(hostNotFound), nil
	}
	if err != nil {
		return nil, err
	}
	return GetHost200JSONResponse(toHost(h)), nil
}

func (s *Server) UpdateHost(ctx context.Context, req UpdateHostRequestObject) (UpdateHostResponseObject, error) {
	h, err := hostFromInput(req.Id, *req.Body)
	if err != nil {
		return UpdateHost400JSONResponse{ErrorJSONResponse(invalidBody(err))}, nil
	}
	var updated model.Host
	_, err = s.applier.Apply(ctx, func(tx *store.Tx) (e error) {
		updated, e = tx.UpdateHost(ctx, h)
		return e
	})
	var taken *store.DomainTakenError
	switch {
	case errors.Is(err, store.ErrNotFound):
		return UpdateHost404JSONResponse(hostNotFound), nil
	case errors.As(err, &taken):
		return UpdateHost409JSONResponse(invalidBody(err)), nil
	case err != nil:
		return nil, err
	}
	return UpdateHost200JSONResponse(toHost(updated)), nil
}

func (s *Server) DeleteHost(ctx context.Context, req DeleteHostRequestObject) (DeleteHostResponseObject, error) {
	_, err := s.applier.Apply(ctx, func(tx *store.Tx) error { return tx.DeleteHost(ctx, req.Id) })
	if errors.Is(err, store.ErrNotFound) {
		return DeleteHost404JSONResponse(hostNotFound), nil
	}
	if err != nil {
		return nil, err
	}
	return DeleteHost204Response{}, nil
}
