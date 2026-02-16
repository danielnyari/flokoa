package server

import (
	"context"
	"strings"

	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/go-logr/logr"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

type claimsContextKey struct{}

// Claims holds the parsed identity claims from a validated OIDC token.
type Claims struct {
	Subject string   `json:"sub"`
	Email   string   `json:"email"`
	Name    string   `json:"name"`
	Groups  []string `json:"groups"`
}

// ClaimsFromContext extracts Claims from the context. Returns nil if not present.
func ClaimsFromContext(ctx context.Context) *Claims {
	c, _ := ctx.Value(claimsContextKey{}).(*Claims)
	return c
}

func contextWithClaims(ctx context.Context, c *Claims) context.Context {
	return context.WithValue(ctx, claimsContextKey{}, c)
}

// AuthInterceptorConfig configures the OIDC auth interceptor.
type AuthInterceptorConfig struct {
	// IssuerURL is the Dex OIDC issuer URL used for discovery.
	IssuerURL string
	// Audience is the expected audience claim in the JWT.
	Audience string
	// PublicMethods are gRPC full method names that skip auth.
	PublicMethods []string
}

// AuthInterceptor holds the initialized OIDC verifier and config.
type AuthInterceptor struct {
	verifier      *oidc.IDTokenVerifier
	publicMethods map[string]bool
	log           logr.Logger
}

// NewAuthInterceptor creates an initialized auth interceptor by performing
// OIDC discovery against the Dex issuer URL.
func NewAuthInterceptor(ctx context.Context, cfg AuthInterceptorConfig, log logr.Logger) (*AuthInterceptor, error) {
	provider, err := oidc.NewProvider(ctx, cfg.IssuerURL)
	if err != nil {
		return nil, err
	}

	verifier := provider.Verifier(&oidc.Config{
		ClientID: cfg.Audience,
	})

	pm := make(map[string]bool, len(cfg.PublicMethods))
	for _, m := range cfg.PublicMethods {
		pm[m] = true
	}

	return &AuthInterceptor{
		verifier:      verifier,
		publicMethods: pm,
		log:           log,
	}, nil
}

// UnaryInterceptor returns a gRPC unary server interceptor that validates
// bearer tokens from the authorization metadata key against Dex's OIDC discovery.
func (a *AuthInterceptor) UnaryInterceptor() grpc.UnaryServerInterceptor {
	return func(
		ctx context.Context,
		req interface{},
		info *grpc.UnaryServerInfo,
		handler grpc.UnaryHandler,
	) (interface{}, error) {
		if a.publicMethods[info.FullMethod] {
			return handler(ctx, req)
		}

		claims, err := a.authenticate(ctx)
		if err != nil {
			return nil, err
		}

		return handler(contextWithClaims(ctx, claims), req)
	}
}

// StreamInterceptor returns a gRPC stream server interceptor that validates
// bearer tokens from the authorization metadata key against Dex's OIDC discovery.
func (a *AuthInterceptor) StreamInterceptor() grpc.StreamServerInterceptor {
	return func(
		srv interface{},
		ss grpc.ServerStream,
		info *grpc.StreamServerInfo,
		handler grpc.StreamHandler,
	) error {
		if a.publicMethods[info.FullMethod] {
			return handler(srv, ss)
		}

		claims, err := a.authenticate(ss.Context())
		if err != nil {
			return err
		}

		wrapped := &authenticatedServerStream{
			ServerStream: ss,
			ctx:          contextWithClaims(ss.Context(), claims),
		}
		return handler(srv, wrapped)
	}
}

// authenticate extracts and validates the bearer token from gRPC metadata.
func (a *AuthInterceptor) authenticate(ctx context.Context) (*Claims, error) {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return nil, status.Error(codes.Unauthenticated, "missing metadata")
	}

	values := md.Get("authorization")
	if len(values) == 0 {
		return nil, status.Error(codes.Unauthenticated, "missing authorization header")
	}

	token := values[0]
	if !strings.HasPrefix(token, "Bearer ") {
		return nil, status.Error(codes.Unauthenticated, "invalid authorization format")
	}
	rawToken := strings.TrimPrefix(token, "Bearer ")

	idToken, err := a.verifier.Verify(ctx, rawToken)
	if err != nil {
		a.log.V(1).Info("Token verification failed", "error", err)
		return nil, status.Error(codes.Unauthenticated, "invalid token")
	}

	var claims Claims
	if err := idToken.Claims(&claims); err != nil {
		a.log.Error(err, "Failed to parse token claims")
		return nil, status.Error(codes.Unauthenticated, "invalid token claims")
	}

	return &claims, nil
}

// authenticatedServerStream wraps a grpc.ServerStream to inject an authenticated context.
type authenticatedServerStream struct {
	grpc.ServerStream
	ctx context.Context
}

func (s *authenticatedServerStream) Context() context.Context {
	return s.ctx
}
