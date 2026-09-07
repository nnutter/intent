package refactor

import (
	"errors"
	"slices"
	"strings"
	"sync"
)

// sortRefactorings orders findings deterministically for stable output.
func sortRefactorings(out []Refactoring) {
	slices.SortFunc(out, func(a, b Refactoring) int {
		if a.Path() != b.Path() {
			return strings.Compare(a.Path(), b.Path())
		}
		if a.Kind() != b.Kind() {
			return strings.Compare(string(a.Kind()), string(b.Kind()))
		}
		return strings.Compare(a.Describe(), b.Describe())
	})
}

// DetectChanges runs Detect over many files concurrently and joins results.
//
// Files that fail to parse do not stop the run; their errors are joined
// and returned alongside the findings from parseable files. Non-Go files
// are skipped.
//
//constable:nonmutating
func DetectChanges(changes []FileChange) ([]Refactoring, error) {
	var mu sync.Mutex
	var out []Refactoring
	var errs []error
	var wg sync.WaitGroup
	for _, ch := range changes {
		if !strings.HasSuffix(ch.Path, ".go") {
			continue
		}
		wg.Go(func() {
			found, err := Detect(ch.Path, ch.Before, ch.After)
			mu.Lock()
			defer mu.Unlock()
			if err != nil {
				errs = append(errs, err)
				return
			}
			out = append(out, found...)
		})
	}
	wg.Wait()
	sortRefactorings(out)
	return out, errors.Join(errs...)
}
