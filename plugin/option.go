package plugin

type Option func(*Descriptor)

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
