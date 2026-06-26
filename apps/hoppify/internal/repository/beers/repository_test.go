package beers

import (
	"strings"
	"testing"
)

func TestTrigramSearchStatementExcludesFTSResults(t *testing.T) {
	t.Parallel()

	statement, args := trigramSearchStatement("punk ipa", 2, []int64{5702, 3691203})

	if !strings.Contains(statement, "beer.untappd_id NOT IN ($3, $4)") {
		t.Fatalf("expected excluded ids in statement, got %q", statement)
	}
	if len(args) != 4 || args[0] != "punk ipa" || args[1] != 2 || args[2] != int64(5702) {
		t.Fatalf("unexpected args: %#v", args)
	}
}
