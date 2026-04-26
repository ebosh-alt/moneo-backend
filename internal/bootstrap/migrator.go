package bootstrap

type Migrator struct {
	Config Config
}

func NewMigrator(cfg Config) (*Migrator, error) {
	if cfg.ServiceName == "" {
		cfg.ServiceName = "moneo-migrator"
	}

	return &Migrator{Config: cfg}, nil
}
