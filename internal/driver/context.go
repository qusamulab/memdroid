package driver

import "context"

// Context returns the lifetime of a bound operation or connection.
func Context(d Driver) context.Context {
	if c, ok := d.(interface{ Context() context.Context }); ok {
		return c.Context()
	}
	return context.Background()
}

// WithContext binds transport calls to ctx when the driver supports cancellation.
func WithContext(d Driver, ctx context.Context) Driver {
	if c, ok := d.(interface{ WithContext(context.Context) Driver }); ok {
		return c.WithContext(ctx)
	}
	return d
}
