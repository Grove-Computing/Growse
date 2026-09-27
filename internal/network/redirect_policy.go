package network

import (
	"context"
	"net/url"
)

type scopedRedirectPolicy struct {
	target   string
	validate func(*url.URL, int) error
}
type redirectPolicyKey struct{}

// WithRedirectPolicy adds a stricter policy to one initial resource URL.
// It does not change subresource loading or the client's existing policy.
func WithRedirectPolicy(ctx context.Context, target string, validate func(*url.URL, int) error) context.Context {
	return context.WithValue(ctx, redirectPolicyKey{}, scopedRedirectPolicy{target, validate})
}
func requestRedirectPolicy(ctx context.Context, target *url.URL) func(*url.URL, int) error {
	policy, _ := ctx.Value(redirectPolicyKey{}).(scopedRedirectPolicy)
	if target != nil && target.String() == policy.target {
		return policy.validate
	}
	return nil
}
