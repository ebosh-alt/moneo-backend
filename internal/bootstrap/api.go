// Package bootstrap is the composition root for runnable Moneo processes.
package bootstrap

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	appaccounting "moneo/internal/app/accounting"
	appcatalog "moneo/internal/app/catalog"
	appidentity "moneo/internal/app/identity"
	"moneo/internal/infra/clock"
	"moneo/internal/infra/idgen"
	"moneo/internal/infra/postgres"
	"moneo/internal/infra/security"
	transporthttp "moneo/internal/transport/http"

	"github.com/jackc/pgx/v5/pgxpool"
)

const defaultAPIListenAddr = ":8080"

type Config struct {
	ServiceName string
	ListenAddr  string
}

type API struct {
	Config Config
	server *http.Server
	pool   *pgxpool.Pool
}

func NewAPI(cfg Config) (*API, error) {
	if cfg.ServiceName == "" {
		cfg.ServiceName = "moneo-api"
	}
	if cfg.ListenAddr == "" {
		cfg.ListenAddr = defaultAPIListenAddr
	}

	tokenCfg, err := LoadAuthTokenConfigFromEnv()
	if err != nil {
		return nil, fmt.Errorf("load auth token config: %w", err)
	}

	postgresCfg, err := LoadPostgresConfigFromEnv()
	if err != nil {
		return nil, fmt.Errorf("load postgres config: %w", err)
	}

	pool, err := pgxpool.New(context.Background(), postgresCfg.DSN)
	if err != nil {
		return nil, fmt.Errorf("open postgres pool: %w", err)
	}
	if err := pool.Ping(context.Background()); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping postgres: %w", err)
	}

	systemClock := clock.NewSystemClock()
	ids := idgen.NewUUIDGenerator()
	passwordHasher := security.NewArgon2IDHasher(security.DefaultArgon2IDConfig())

	users := postgres.NewAuthUserRepository(pool)
	sessions := postgres.NewAuthSessionRepository(pool)
	oneTimeTokens := postgres.NewAuthOneTimeTokenRepository(pool)
	txManager := postgres.NewTxManager(pool)

	authService := appidentity.NewAuthService(
		users,
		passwordHasher,
		ids,
		systemClock,
	)

	tokenService, err := security.NewTokenService(security.TokenServiceConfig{
		AccessTokenTTL:  tokenCfg.AccessTokenTTL,
		RefreshTokenTTL: tokenCfg.RefreshTokenTTL,
		JWTSecret:       tokenCfg.JWTSecret,
	}, systemClock)
	if err != nil {
		pool.Close()
		return nil, fmt.Errorf("new token service: %w", err)
	}

	authFlowService := appidentity.NewAuthFlowService(
		authService,
		sessions,
		ids,
		tokenService,
		systemClock,
	)

	authPostMVPService, err := appidentity.NewAuthPostMVPService(
		users,
		sessions,
		oneTimeTokens,
		tokenService,
		ids,
		passwordHasher,
		txManager,
		systemClock,
		nil,
		nil,
		appidentity.DefaultAuthPostMVPConfig(),
	)
	if err != nil {
		pool.Close()
		return nil, fmt.Errorf("new post-mvp auth service: %w", err)
	}

	accessAuthService := appidentity.NewAccessAuthService(tokenService, users, sessions)
	authMiddleware := transporthttp.NewAuthMiddleware(accessAuthService)
	authHandler := transporthttp.NewAuthHandler(authFlowService, authPostMVPService)
	accountRepository := postgres.NewAccountRepository(pool)
	accountCreateService := appaccounting.NewCreateAccountService(accountRepository, ids, systemClock)
	accountGetService := appaccounting.NewGetAccountService(accountRepository)
	accountListService := appaccounting.NewListAccountsService(accountRepository)
	accountSummaryService := appaccounting.NewGetAccountsSummaryService(accountRepository)
	accountUpdateService := appaccounting.NewUpdateAccountService(accountRepository, systemClock)

	categoryQueryService := appcatalog.NewCategoryQueryService(emptyCategoryRepository{})
	subcategoryQueryService := appcatalog.NewSubcategoryQueryService(emptySubcategoryRepository{})
	catalogHandler := transporthttp.NewCatalogHandler(
		accountCreateService,
		accountGetService,
		accountListService,
		accountSummaryService,
		accountUpdateService,
		categoryQueryService,
		categoryQueryService,
		subcategoryQueryService,
		subcategoryQueryService,
	)
	router := transporthttp.NewRouterWithOptions(authHandler, transporthttp.RouterOptions{
		AuthMiddleware: authMiddleware,
		CatalogHandler: catalogHandler,
	})

	server := &http.Server{
		Addr:              cfg.ListenAddr,
		Handler:           router,
		ReadHeaderTimeout: 10 * time.Second,
	}

	return &API{
		Config: cfg,
		server: server,
		pool:   pool,
	}, nil
}

func (a *API) Run(ctx context.Context) error {
	if a == nil || a.server == nil {
		return errors.New("api server is not initialized")
	}
	if a.pool == nil {
		return errors.New("postgres pool is not initialized")
	}

	defer a.pool.Close()

	serverErrCh := make(chan error, 1)
	go func() {
		err := a.server.ListenAndServe()
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			serverErrCh <- err
			return
		}
		serverErrCh <- nil
	}()

	select {
	case err := <-serverErrCh:
		return err
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		if err := a.server.Shutdown(shutdownCtx); err != nil {
			return fmt.Errorf("shutdown api server: %w", err)
		}

		return <-serverErrCh
	}
}
