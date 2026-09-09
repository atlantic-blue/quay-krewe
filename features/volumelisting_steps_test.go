package features_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/cucumber/godog"
)

// Steps for the verb that says what a volume holds.
//
// They run the real tool in its own process, because the shape of the answer is what is specified: a
// directory on the first line and a table under it, and a line that is one line inside the test
// process can be two on the caller's screen.
//
// The directory the files go into is the one the tool itself names, rather than a path this file
// builds. A listing that read a directory nobody copies into would pass against a path assembled
// correctly and mounted nowhere.

func initializeVolumeListingSteps(sc *godog.ScenarioContext) {
	sc.Step(`^the caller lists the volume "([^"]*)"$`, func(ctx context.Context, address string) error {
		return runTool(ctx, "volume", "list", address)
	})

	// A name ending in a slash is a folder, so one step makes both. The rows are written in an order
	// that is not the sorted one, which is what makes the sorted listing worth asserting.
	sc.Step(`^that directory holds$`, func(ctx context.Context, holds *godog.Table) error {
		named := theDirectoryNamed(ctx)
		for _, row := range holds.Rows[1:] {
			name := row.Cells[0].Value
			if folder, is := strings.CutSuffix(name, "/"); is {
				if err := os.MkdirAll(filepath.Join(named, folder), 0o777); err != nil {
					return err
				}
				continue
			}
			if err := os.WriteFile(filepath.Join(named, name), []byte(row.Cells[1].Value), 0o666); err != nil {
				return err
			}
		}
		return nil
	})

	sc.Step(`^the listing reads$`, func(ctx context.Context, want *godog.Table) error {
		printed := listingRows(ctx)
		if len(printed) != len(want.Rows) {
			return fmt.Errorf("the listing has %d rows, want %d:\n%s",
				len(printed), len(want.Rows), toolFrom(ctx).stdout)
		}
		for at, row := range want.Rows {
			// A folder carries no size, and an empty cell is how a table says so.
			wanted := make([]string, 0, len(row.Cells))
			for _, cell := range row.Cells {
				if strings.TrimSpace(cell.Value) != "" {
					wanted = append(wanted, cell.Value)
				}
			}
			if !slices.Equal(printed[at], wanted) {
				return fmt.Errorf("row %d reads %v, want %v:\n%s", at, printed[at], wanted, toolFrom(ctx).stdout)
			}
		}
		return nil
	})
}

// listingRows is the table the tool printed, one row of cells per line.
//
// The first line is the directory the listing read, which is asserted by the scenarios about the
// path rather than by the ones about the names.
func listingRows(ctx context.Context) [][]string {
	said := toolFrom(ctx).stdout
	_, under, _ := strings.Cut(said, "\n")
	rows := make([][]string, 0, 4)
	for _, line := range strings.Split(under, "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		rows = append(rows, strings.Fields(line))
	}
	return rows
}
