// Package bootstrap is the composition root for runnable Moneo processes.
package bootstrap

type Config struct {
	ServiceName string
}

type API struct {
	Config Config
}

func NewAPI(cfg Config) (*API, error) {
	if cfg.ServiceName == "" {
		cfg.ServiceName = "moneo-api"
	}

	return &API{Config: cfg}, nil
}
