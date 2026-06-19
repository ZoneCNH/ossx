package ossx

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

// envPrefix is the canonical configuration environment variable prefix.
// Composition roots load these from operator-owned secret material, such as
// sre/secrets/env/dev.md, before constructing ossx.Config. ossx never imports
// a config loader (BR-002); ConfigFromEnv is a convenience that reads plain
// os.Environ.
const envPrefix = "FOUNDATIONX_OSSX_"

// ConfigFromEnv builds a Config from FOUNDATIONX_OSSX_* environment variables.
//
// Recognized variables:
//
//	FOUNDATIONX_OSSX_ENDPOINT            (required)
//	FOUNDATIONX_OSSX_REGION              (required)
//	FOUNDATIONX_OSSX_BUCKET              (required)
//	FOUNDATIONX_OSSX_ACCESS_KEY          (required; secret)
//	FOUNDATIONX_OSSX_SECRET_KEY          (required; secret)
//	FOUNDATIONX_OSSX_USE_SSL             (default true)
//	FOUNDATIONX_OSSX_CNAME               (optional)
//	FOUNDATIONX_OSSX_OPERATION_TIMEOUT   (default 30s)
//	FOUNDATIONX_OSSX_CONNECT_TIMEOUT     (default 5s)
//	FOUNDATIONX_OSSX_PRESIGN_MAX_TTL     (default 15m; capped at MaxAllowedPresignTTL)
//	FOUNDATIONX_OSSX_MULTIPART_MIN_PART  (default 8MiB)
//	FOUNDATIONX_OSSX_MULTIPART_MAX_PARTS (default 10000)
//
// Missing required variables yield an *Error of kind config listing what's
// missing. The returned Config is validated (Validate) before return.
func ConfigFromEnv() (Config, error) {
	var missing []string
	get := func(key string) string { return os.Getenv(envPrefix + key) }
	require := func(key string) string {
		v := get(key)
		if v == "" {
			missing = append(missing, envPrefix+key)
		}
		return v
	}

	cfg := Config{
		Endpoint:  require("ENDPOINT"),
		Region:    require("REGION"),
		Bucket:    require("BUCKET"),
		AccessKey: require("ACCESS_KEY"),
		SecretKey: require("SECRET_KEY"),
		UseSSL:    boolEnvDefault("USE_SSL", true),
		CNAME:     get("CNAME"),
	}
	if len(missing) > 0 {
		return Config{}, newError(ErrorKindConfig, "env", fmt.Sprintf("missing required environment variables: %s", strings.Join(missing, ", ")))
	}

	cfg.Timeouts.Operation = durationEnvDefault("OPERATION_TIMEOUT", 30*time.Second)
	cfg.Timeouts.Connect = durationEnvDefault("CONNECT_TIMEOUT", 5*time.Second)
	cfg.Presign.MaxTTL = durationEnvDefault("PRESIGN_MAX_TTL", MaxAllowedPresignTTL)
	cfg.Presign.AllowedOperations = []PresignOperation{PresignGet, PresignPut}
	cfg.Multipart.MinPartSize = int64EnvDefault("MULTIPART_MIN_PART", 8*1024*1024)
	cfg.Multipart.MaxParts = int(int64EnvDefault("MULTIPART_MAX_PARTS", 10000))
	cfg.Retry = DefaultRetry()
	cfg.Checksum.Algorithms = []ChecksumAlgorithm{ChecksumSHA256}

	cfg = cfg.withDefaults()
	if err := cfg.Validate(); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

// env helpers --------------------------------------------------------------

func boolEnvDefault(key string, def bool) bool {
	v := os.Getenv(envPrefix + key)
	if v == "" {
		return def
	}
	b, err := strconv.ParseBool(v)
	if err != nil {
		return def
	}
	return b
}

func durationEnvDefault(key string, def time.Duration) time.Duration {
	v := os.Getenv(envPrefix + key)
	if v == "" {
		return def
	}
	d, err := time.ParseDuration(v)
	if err != nil {
		return def
	}
	return d
}

func int64EnvDefault(key string, def int64) int64 {
	v := os.Getenv(envPrefix + key)
	if v == "" {
		return def
	}
	n, err := strconv.ParseInt(v, 10, 64)
	if err != nil {
		return def
	}
	return n
}
