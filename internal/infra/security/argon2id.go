package security

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"golang.org/x/crypto/argon2"
)

var (
	ErrInvalidHashFormat = errors.New("invalid password hash format")
	ErrIncompatibleHash  = errors.New("incompatible password hash")
)

type Argon2IDConfig struct {
	Time       uint32
	Memory     uint32
	Threads    uint8
	SaltLength uint32
	KeyLength  uint32
}

func DefaultArgon2IDConfig() Argon2IDConfig {
	return Argon2IDConfig{
		Time:       3,
		Memory:     64 * 1024,
		Threads:    2,
		SaltLength: 16,
		KeyLength:  32,
	}
}

type Argon2IDHasher struct {
	config Argon2IDConfig
}

func NewArgon2IDHasher(config Argon2IDConfig) *Argon2IDHasher {
	defaults := DefaultArgon2IDConfig()
	if config.Time == 0 {
		config.Time = defaults.Time
	}
	if config.Memory == 0 {
		config.Memory = defaults.Memory
	}
	if config.Threads == 0 {
		config.Threads = defaults.Threads
	}
	if config.SaltLength == 0 {
		config.SaltLength = defaults.SaltLength
	}
	if config.KeyLength == 0 {
		config.KeyLength = defaults.KeyLength
	}

	return &Argon2IDHasher{config: config}
}

func (h *Argon2IDHasher) Hash(password string) (string, error) {
	salt := make([]byte, h.config.SaltLength)
	if _, err := rand.Read(salt); err != nil {
		return "", fmt.Errorf("read salt: %w", err)
	}

	hash := argon2.IDKey(
		[]byte(password),
		salt,
		h.config.Time,
		h.config.Memory,
		h.config.Threads,
		h.config.KeyLength,
	)

	b64Salt := base64.RawStdEncoding.EncodeToString(salt)
	b64Hash := base64.RawStdEncoding.EncodeToString(hash)

	return fmt.Sprintf(
		"$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s",
		argon2.Version,
		h.config.Memory,
		h.config.Time,
		h.config.Threads,
		b64Salt,
		b64Hash,
	), nil
}

func (h *Argon2IDHasher) Verify(password string, encodedHash string) (bool, error) {
	params, salt, expectedHash, err := decodeArgon2IDHash(encodedHash)
	if err != nil {
		return false, err
	}

	computedHash := argon2.IDKey(
		[]byte(password),
		salt,
		params.Time,
		params.Memory,
		params.Threads,
		uint32(len(expectedHash)),
	)

	if subtle.ConstantTimeCompare(computedHash, expectedHash) == 1 {
		return true, nil
	}

	return false, nil
}

func decodeArgon2IDHash(encodedHash string) (Argon2IDConfig, []byte, []byte, error) {
	parts := strings.Split(encodedHash, "$")
	if len(parts) != 6 || parts[1] != "argon2id" {
		return Argon2IDConfig{}, nil, nil, ErrInvalidHashFormat
	}

	versionPart := strings.TrimPrefix(parts[2], "v=")
	version, err := strconv.Atoi(versionPart)
	if err != nil {
		return Argon2IDConfig{}, nil, nil, ErrInvalidHashFormat
	}
	if version != argon2.Version {
		return Argon2IDConfig{}, nil, nil, ErrIncompatibleHash
	}

	var cfg Argon2IDConfig
	_, err = fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &cfg.Memory, &cfg.Time, &cfg.Threads)
	if err != nil {
		return Argon2IDConfig{}, nil, nil, ErrInvalidHashFormat
	}

	salt, err := base64.RawStdEncoding.DecodeString(parts[4])
	if err != nil {
		return Argon2IDConfig{}, nil, nil, ErrInvalidHashFormat
	}

	hash, err := base64.RawStdEncoding.DecodeString(parts[5])
	if err != nil {
		return Argon2IDConfig{}, nil, nil, ErrInvalidHashFormat
	}

	cfg.SaltLength = uint32(len(salt))
	cfg.KeyLength = uint32(len(hash))

	return cfg, salt, hash, nil
}
