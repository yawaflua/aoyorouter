package config

import (
	"os"

	"github.com/ilyakaznacheev/cleanenv"
	"github.com/yawaflua/aoyorouter/internal/adapter/postgres"
)

type C struct {
	Env string `env:"ENV" env-default:"dev"`

	InitialPassword      string `env:"INITIAL_PASSWORD" env-default:""`
	WarpLimit            int    `env:"WARP_LIMIT" env-default:"10"`
	NotUseCloudflare     bool   `env:"ILL_NOT_USE_CLOUDFLARE_REALLY_NOT_NEEDED" env-default:"false"`
	EnableEffortPresets  bool   `env:"ENABLE_EFFORT_PRESETS" env-default:"false"`

	GRPC     GRPCConfig
	HTTP     HTTPConfig
	Postgres postgres.Config
}

type GRPCConfig struct {
	Host string `env:"GRPC_HOST" env-default:"localhost"`
	Port int    `env:"GRPC_PORT" env-default:"9090"`
}

type HTTPConfig struct {
	Host             string `env:"HTTP_HOST" env-default:"0.0.0.0"`
	Port             int    `env:"HTTP_PORT" env-default:"8080"`
	CursorServerPort int    `env:"CURSOR_SERVER_PORT" env-default:"8787"`
}

func MustLoad() *C {
	var cfg C

	if err := cleanenv.ReadConfig(".env", &cfg); err != nil && !os.IsNotExist(err) {
		panic(err)
	}
	err := cleanenv.ReadEnv(&cfg)
	if err != nil {
		panic(err)
	}

	return &cfg
}
