package plugin

// Option configures a contribution Descriptor at registration time.
type Option func(*Descriptor)

// WithServiceOnly marks a contribution as service-only: it stays registered
// and executable for host/daemon routing, but interactive UI lists omit it
// unless a live serve/daemon socket is attached ([ServiceAttached]).
func WithServiceOnly() Option {
	return func(d *Descriptor) { d.ServiceOnly = true }
}

func applyOptions(d *Descriptor, opts []Option) {
	for _, opt := range opts {
		if opt != nil {
			opt(d)
		}
	}
}
