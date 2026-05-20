package eventschema

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"github.com/sanskarpan/PayGate/internal/outbox"
)

// BootstrapFromFixtures ensures the checked-in schema fixtures are present in
// the registry and that a clean environment has an active version per subject.
func (s *Service) BootstrapFromFixtures(ctx context.Context, rootDir, owner string) error {
	entries, err := os.ReadDir(rootDir)
	if err != nil {
		return err
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		subject := entry.Name()
		subjectDir := filepath.Join(rootDir, subject)

		if _, err := s.repo.GetSchema(ctx, subject); err != nil {
			if !errors.Is(err, ErrSchemaNotFound) {
				return err
			}
			if _, err := s.repo.CreateSchema(ctx, CreateSchemaInput{
				Subject:    subject,
				EventType:  subject,
				TopicName:  outbox.TopicForEvent(subject),
				Owner:      owner,
				ReviewLink: filepath.ToSlash(filepath.Join("schemas", "events", subject)),
			}); err != nil {
				return fmt.Errorf("create schema %s: %w", subject, err)
			}
		}

		versions, err := fixtureVersions(subjectDir)
		if err != nil {
			return fmt.Errorf("load schema fixture versions for %s: %w", subject, err)
		}
		if len(versions) == 0 {
			continue
		}

		activeVersion := ""
		hasActive := false
		active, err := s.repo.GetActiveVersion(ctx, subject)
		switch {
		case err == nil:
			activeVersion = active.Version
			hasActive = true
		case errors.Is(err, ErrNoActiveSchemaVersion):
		default:
			return err
		}

		for i, version := range versions {
			if _, err := s.repo.GetVersion(ctx, subject, version); err != nil {
				if !errors.Is(err, ErrSchemaVersionNotFound) {
					return err
				}
				document, sample, err := loadFixtureVersion(subjectDir, version)
				if err != nil {
					return fmt.Errorf("load schema fixture %s/%s: %w", subject, version, err)
				}
				if _, _, err := s.RegisterVersion(ctx, CreateVersionInput{
					Subject:       subject,
					Version:       version,
					Schema:        document,
					SamplePayload: sample,
					ReviewLink:    filepath.ToSlash(filepath.Join("schemas", "events", subject, version+".schema.json")),
				}); err != nil {
					return fmt.Errorf("register schema fixture %s/%s: %w", subject, version, err)
				}
			}

			// In a clean registry, activate the earliest fixture first so later
			// versions register against a real baseline instead of "no active".
			if !hasActive && i == 0 {
				if _, err := s.ActivateVersion(ctx, ActivateVersionInput{Subject: subject, Version: version}); err != nil {
					return fmt.Errorf("activate bootstrap baseline %s/%s: %w", subject, version, err)
				}
				activeVersion = version
				hasActive = true
			}
		}

		latest := versions[len(versions)-1]
		if hasActive && activeVersion != latest {
			if _, err := s.ActivateVersion(ctx, ActivateVersionInput{Subject: subject, Version: latest}); err != nil {
				return fmt.Errorf("activate latest bootstrap version %s/%s: %w", subject, latest, err)
			}
		}
	}
	return nil
}

func fixtureVersions(subjectDir string) ([]string, error) {
	entries, err := os.ReadDir(subjectDir)
	if err != nil {
		return nil, err
	}

	seen := map[string]bool{}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		switch {
		case strings.HasSuffix(name, ".schema.json"):
			seen[strings.TrimSuffix(name, ".schema.json")] = true
		case strings.HasSuffix(name, ".sample.json"):
			seen[strings.TrimSuffix(name, ".sample.json")] = true
		}
	}

	versions := make([]string, 0, len(seen))
	for version := range seen {
		schemaPath := filepath.Join(subjectDir, version+".schema.json")
		samplePath := filepath.Join(subjectDir, version+".sample.json")
		if _, err := os.Stat(schemaPath); err != nil {
			return nil, fmt.Errorf("missing schema fixture %s", schemaPath)
		}
		if _, err := os.Stat(samplePath); err != nil {
			return nil, fmt.Errorf("missing sample fixture %s", samplePath)
		}
		versions = append(versions, version)
	}
	slices.SortFunc(versions, compareVersions)
	return versions, nil
}

func loadFixtureVersion(subjectDir, version string) (Document, map[string]any, error) {
	var document Document
	var sample map[string]any

	schemaRaw, err := fixtureReadFile(subjectDir, version+".schema.json")
	if err != nil {
		return Document{}, nil, err
	}
	if err := json.Unmarshal(schemaRaw, &document); err != nil {
		return Document{}, nil, err
	}

	sampleRaw, err := fixtureReadFile(subjectDir, version+".sample.json")
	if err != nil {
		return Document{}, nil, err
	}
	if err := json.Unmarshal(sampleRaw, &sample); err != nil {
		return Document{}, nil, err
	}

	return document, sample, nil
}

func fixtureReadFile(dir, name string) ([]byte, error) {
	path := filepath.Join(dir, filepath.Clean(name))
	// #nosec G304 -- schema fixtures are repo-local files resolved from controlled directories.
	return os.ReadFile(path)
}

func compareVersions(a, b string) int {
	ap := strings.Split(a, ".")
	bp := strings.Split(b, ".")
	maxParts := len(ap)
	if len(bp) > maxParts {
		maxParts = len(bp)
	}
	for i := 0; i < maxParts; i++ {
		ai := 0
		bi := 0
		if i < len(ap) {
			ai, _ = strconv.Atoi(ap[i])
		}
		if i < len(bp) {
			bi, _ = strconv.Atoi(bp[i])
		}
		if ai < bi {
			return -1
		}
		if ai > bi {
			return 1
		}
	}
	return 0
}
