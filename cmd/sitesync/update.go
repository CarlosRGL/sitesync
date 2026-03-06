package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

const latestReleaseURL = "https://api.github.com/repos/CarlosRGL/sitesync/releases/latest"

type releaseInfo struct {
	TagName string `json:"tag_name"`
}

func latestUpdateNotice() string {
	if !shouldCheckForUpdates(version) {
		return ""
	}

	ctx, cancel := context.WithTimeout(context.Background(), 1500*time.Millisecond)
	defer cancel()

	latest, err := fetchLatestReleaseVersion(ctx)
	if err != nil || latest == "" {
		return ""
	}

	cmp, err := compareVersions(normalizeVersion(version), normalizeVersion(latest))
	if err != nil || cmp >= 0 {
		return ""
	}

	return fmt.Sprintf("sitesync %s available (current %s). Update: curl -fsSL https://github.com/CarlosRGL/sitesync/releases/latest/download/install.sh | sh", normalizeVersion(latest), normalizeVersion(version))
}

func shouldCheckForUpdates(v string) bool {
	n := normalizeVersion(v)
	if n == "" {
		return false
	}
	return strings.IndexFunc(n, func(r rune) bool { return !(r == '.' || (r >= '0' && r <= '9')) }) == -1
}

func fetchLatestReleaseVersion(ctx context.Context) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, latestReleaseURL, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "sitesync-update-check")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("latest release lookup failed: %s", resp.Status)
	}

	var release releaseInfo
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return "", err
	}
	return release.TagName, nil
}

func normalizeVersion(v string) string {
	return strings.TrimPrefix(strings.TrimSpace(v), "v")
}

func compareVersions(a, b string) (int, error) {
	parse := func(v string) ([]int, error) {
		parts := strings.Split(v, ".")
		out := make([]int, len(parts))
		for i, part := range parts {
			n, err := strconv.Atoi(part)
			if err != nil {
				return nil, fmt.Errorf("invalid version %q", v)
			}
			out[i] = n
		}
		return out, nil
	}

	ap, err := parse(a)
	if err != nil {
		return 0, err
	}
	bp, err := parse(b)
	if err != nil {
		return 0, err
	}

	maxLen := len(ap)
	if len(bp) > maxLen {
		maxLen = len(bp)
	}
	for len(ap) < maxLen {
		ap = append(ap, 0)
	}
	for len(bp) < maxLen {
		bp = append(bp, 0)
	}

	for i := 0; i < maxLen; i++ {
		switch {
		case ap[i] < bp[i]:
			return -1, nil
		case ap[i] > bp[i]:
			return 1, nil
		}
	}
	return 0, nil
}

func printUpdateNotice(w io.Writer) {
	if notice := latestUpdateNotice(); notice != "" {
		fmt.Fprintln(w, notice)
	}
}
