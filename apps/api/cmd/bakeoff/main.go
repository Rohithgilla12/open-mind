// Command bakeoff runs the extractor bake-off: for each URL in a corpus it
// runs the selected extractors and prints a markdown table of the results.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/rohithgilla12/openmind/api/internal/enrich"
)

type corpusEntry struct {
	URL  string `json:"url"`
	Type string `json:"type"`
}

const perURLTimeout = 20 * time.Second

func main() {
	corpusPath := flag.String("corpus", "testdata/bakeoff.json", "path to corpus JSON")
	extractorName := flag.String("extractor", "all", "extractor to run: trafilatura, readability, jina, or all")
	flag.Parse()

	if err := run(*corpusPath, *extractorName); err != nil {
		fmt.Fprintln(os.Stderr, "bakeoff:", err)
		os.Exit(1)
	}
}

func run(corpusPath, extractorName string) error {
	raw, err := os.ReadFile(corpusPath)
	if err != nil {
		return fmt.Errorf("reading corpus: %w", err)
	}
	var corpus []corpusEntry
	if err := json.Unmarshal(raw, &corpus); err != nil {
		return fmt.Errorf("parsing corpus: %w", err)
	}

	extractors := selectExtractors(extractorName)
	if len(extractors) == 0 {
		return fmt.Errorf("unknown extractor %q", extractorName)
	}

	fmt.Println("# Extraction bake-off")
	fmt.Println()
	fmt.Printf("Corpus: `%s` (%d URLs). Timeout: %s per URL×extractor. Run: %s\n",
		corpusPath, len(corpus), perURLTimeout, time.Now().UTC().Format("2006-01-02 15:04 MST"))
	fmt.Println()

	for _, entry := range corpus {
		fmt.Printf("## %s (`%s`)\n\n", entry.URL, entry.Type)
		fmt.Println("| Extractor | Title | Body len | First 200 chars | Image | Latency (ms) | Error |")
		fmt.Println("|---|---|---|---|---|---|---|")
		for _, ex := range extractors {
			printRow(ex, entry.URL)
		}
		fmt.Println()
	}
	return nil
}

func selectExtractors(name string) []enrich.Extractor {
	traf := enrich.NewTrafilatura(nil)
	read := enrich.NewReadability(nil)
	jina := enrich.NewJina(nil, os.Getenv("JINA_API_KEY"))
	switch name {
	case "all":
		return []enrich.Extractor{traf, read, jina}
	case "trafilatura":
		return []enrich.Extractor{traf}
	case "readability":
		return []enrich.Extractor{read}
	case "jina":
		return []enrich.Extractor{jina}
	default:
		return nil
	}
}

func printRow(ex enrich.Extractor, url string) {
	ctx, cancel := context.WithTimeout(context.Background(), perURLTimeout)
	defer cancel()

	start := time.Now()
	res, err := ex.Extract(ctx, url)
	latency := time.Since(start).Milliseconds()

	if err != nil {
		fmt.Printf("| %s | | | | | %d | %s |\n", ex.Name(), latency, cell(err.Error()))
		return
	}
	image := "n"
	if res.LeadImageURL != "" {
		image = "y"
	}
	fmt.Printf("| %s | %s | %d | %s | %s | %d | |\n",
		ex.Name(), cell(res.Title), len(res.Body), cell(firstChars(res.Body, 200)), image, latency)
}

// firstChars returns up to n runes of s.
func firstChars(s string, n int) string {
	r := []rune(s)
	if len(r) > n {
		return string(r[:n])
	}
	return string(r)
}

// cell escapes a value for a single markdown table cell: collapses newlines and
// escapes pipes.
func cell(s string) string {
	s = strings.ReplaceAll(s, "\r", " ")
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.ReplaceAll(s, "\t", " ")
	s = strings.ReplaceAll(s, "|", "\\|")
	return strings.TrimSpace(s)
}
