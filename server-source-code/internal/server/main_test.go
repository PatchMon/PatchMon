package server

import (
	"os"
	"testing"

	"github.com/PatchMon/PatchMon/server-source-code/internal/pgtest"
)

func TestMain(m *testing.M) { os.Exit(pgtest.Main(m)) }
