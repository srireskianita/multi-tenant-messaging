package config

import (
    "log"

    "github.com/spf13/viper"
)

type Config struct {
    RabbitMQ struct {
        URL string
    }
    Database struct {
        URL string
    }
    Workers int
}

func Load() *Config {
    viper.SetConfigName("config")
    viper.SetConfigType("yaml")
    viper.AddConfigPath("config")

    err := viper.ReadInConfig()
    if err != nil {
        log.Fatalf("Error reading config file: %v", err)
    }

    var cfg Config
    err = viper.Unmarshal(&cfg)
    if err != nil {
        log.Fatalf("Error unmarshaling config: %v", err)
    }

    return &cfg
}
