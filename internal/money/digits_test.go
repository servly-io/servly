package money

import (
	"os"
	"regexp"
	"slices"
	"strings"
	"testing"
)

// The web app formats and parses prices with a copy of the digits table; a
// difference would make a price typed in the browser land 100 times off.
func TestWebDigitsMatch(t *testing.T) {
	src, err := os.ReadFile("../../web/src/shared/money.ts")
	if err != nil {
		t.Fatal(err)
	}
	set := func(name string) []string {
		m := regexp.MustCompile(`const ` + name + ` = new Set\(\[([^\]]*)\]\)`).FindStringSubmatch(string(src))
		if m == nil {
			t.Fatalf("%s not found in money.ts", name)
		}
		codes := regexp.MustCompile(`[A-Z]{3}`).FindAllString(m[1], -1)
		slices.Sort(codes)
		return codes
	}
	keys := func(m map[Currency]bool) []string {
		var out []string
		for c := range m {
			out = append(out, string(c))
		}
		slices.Sort(out)
		return out
	}
	if web, gox := set("zeroDecimal"), keys(zeroDecimal); !slices.Equal(web, gox) {
		t.Errorf("zero-decimal currencies differ:\n web %s\n go  %s", strings.Join(web, " "), strings.Join(gox, " "))
	}
	if web, gox := set("threeDecimal"), keys(threeDecimal); !slices.Equal(web, gox) {
		t.Errorf("three-decimal currencies differ:\n web %s\n go  %s", strings.Join(web, " "), strings.Join(gox, " "))
	}
	if Digits("IDR") != 0 || Format(17000, "IDR") != "17000 IDR" || Format(950, "EUR") != "9.50 EUR" {
		t.Errorf("IDR digits %d, %q, %q", Digits("IDR"), Format(17000, "IDR"), Format(950, "EUR"))
	}
}
