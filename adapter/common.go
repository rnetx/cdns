package adapter

type Starter interface {
	Start() error
}

type Closer interface {
	Close() error
}

func Start(v any) error {
	starter, isStarter := v.(Starter)
	if isStarter {
		return starter.Start()
	}
	return nil
}

func Close(v any) error {
	closer, isCloser := v.(Closer)
	if isCloser {
		return closer.Close()
	}
	return nil
}
