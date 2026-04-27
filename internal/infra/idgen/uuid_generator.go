package idgen

import (
	"github.com/google/uuid"

	"moneo/internal/domain/shared"
)

type UUIDGenerator struct{}

func NewUUIDGenerator() UUIDGenerator {
	return UUIDGenerator{}
}

func (UUIDGenerator) NewUserID() shared.UserID {
	return shared.UserID(uuid.NewString())
}

func (UUIDGenerator) NewSessionID() shared.SessionID {
	return shared.SessionID(uuid.NewString())
}

func (UUIDGenerator) NewOneTimeTokenID() shared.OneTimeTokenID {
	return shared.OneTimeTokenID(uuid.NewString())
}

func (UUIDGenerator) NewAccountID() shared.AccountID {
	return shared.AccountID(uuid.NewString())
}
