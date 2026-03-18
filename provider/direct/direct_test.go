package direct_test

import (
	"github.com/imgajeed76/pgit/v4/provider"
	"github.com/imgajeed76/pgit/v4/provider/direct"
)

// Compile-time interface satisfaction check.
var _ provider.Provider = (*direct.Provider)(nil)
