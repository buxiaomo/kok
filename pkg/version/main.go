package version

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
)

var List = map[string]map[string]string{
	"v1.14": {
		"kubernetes":     "v1.14.10",
		"etcd":           "3.3.10-0",
		"containerd":     "1.5.13",
		"runc":           "1.1.2",
		"pause":          "3.1",
		"coredns":        "1.3.1",
		"metrics-server": "0.5.2",
	},
	"v1.15": {
		"kubernetes":     "v1.15.12",
		"etcd":           "3.3.10-0",
		"containerd":     "1.5.13",
		"runc":           "1.1.2",
		"pause":          "3.1",
		"coredns":        "1.3.1",
		"metrics-server": "0.5.2",
	},
	"v1.16": {
		"kubernetes":     "v1.16.15",
		"etcd":           "3.3.10-0",
		"containerd":     "1.5.13",
		"runc":           "1.1.2",
		"pause":          "3.1",
		"coredns":        "1.6.2",
		"metrics-server": "0.5.2",
	},
	"v1.17": {
		"kubernetes":     "v1.17.17",
		"etcd":           "3.4.3-0",
		"containerd":     "1.5.13",
		"runc":           "1.1.2",
		"pause":          "3.1",
		"coredns":        "1.6.5",
		"metrics-server": "0.5.2",
	},
	"v1.18": {
		"kubernetes":     "v1.18.20",
		"etcd":           "3.4.3-0",
		"containerd":     "1.5.13",
		"runc":           "1.1.2",
		"pause":          "3.2",
		"coredns":        "1.6.7",
		"metrics-server": "0.5.2",
	},
	"v1.19": {
		"kubernetes":     "v1.19.16",
		"etcd":           "3.4.13-0",
		"containerd":     "1.5.13",
		"runc":           "1.1.2",
		"pause":          "3.2",
		"coredns":        "1.7.0",
		"metrics-server": "0.7.1",
	},
	"v1.20": {
		"kubernetes":     "v1.20.15",
		"etcd":           "3.4.13-0",
		"containerd":     "1.5.13",
		"runc":           "1.1.2",
		"pause":          "3.2",
		"coredns":        "1.7.0",
		"metrics-server": "0.7.1",
	},
	"v1.21": {
		"kubernetes":     "v1.21.14",
		"etcd":           "3.4.13-0",
		"containerd":     "1.5.13",
		"runc":           "1.1.2",
		"pause":          "3.4.1",
		"coredns":        "1.8.0",
		"metrics-server": "0.7.1",
	},
	"v1.22": {
		"kubernetes":     "v1.22.17",
		"etcd":           "3.5.0-0",
		"containerd":     "1.5.13",
		"runc":           "1.1.2",
		"pause":          "3.5",
		"coredns":        "1.8.4",
		"metrics-server": "0.7.1",
	},
	"v1.23": {
		"kubernetes":     "v1.23.17",
		"etcd":           "3.5.1-0",
		"containerd":     "1.7.0",
		"runc":           "1.1.4",
		"pause":          "3.6",
		"coredns":        "1.8.6",
		"metrics-server": "0.7.1",
	},
	"v1.24": {
		"kubernetes":     "v1.24.17",
		"etcd":           "3.5.3-0",
		"containerd":     "1.7.6",
		"runc":           "1.1.9",
		"pause":          "3.6",
		"coredns":        "1.8.6",
		"metrics-server": "0.7.1",
	},
	"v1.25": {
		"kubernetes":     "v1.25.16",
		"etcd":           "3.5.4-0",
		"containerd":     "1.7.13",
		"runc":           "1.1.12",
		"pause":          "3.8",
		"coredns":        "1.9.3",
		"metrics-server": "0.7.1",
	},
	"v1.26": {
		"kubernetes":     "v1.26.15",
		"etcd":           "3.5.6-0",
		"containerd":     "1.7.18",
		"runc":           "1.1.12",
		"pause":          "3.9",
		"coredns":        "1.9.3",
		"metrics-server": "0.7.1",
	},
	"v1.27": {
		"kubernetes":     "v1.27.15",
		"etcd":           "3.5.7-0",
		"containerd":     "1.7.20",
		"runc":           "1.1.13",
		"pause":          "3.9",
		"coredns":        "1.10.1",
		"metrics-server": "0.7.1",
	},
	"v1.28": {
		"kubernetes":     "v1.28.12",
		"etcd":           "3.5.9-0",
		"containerd":     "1.7.20",
		"runc":           "1.1.13",
		"pause":          "3.9",
		"coredns":        "1.10.1",
		"metrics-server": "0.7.1",
	},
	"v1.29": {
		"kubernetes":     "v1.29.7",
		"etcd":           "3.5.12-0",
		"containerd":     "1.7.20",
		"runc":           "1.1.13",
		"pause":          "3.9",
		"coredns":        "1.11.1",
		"metrics-server": "0.7.1",
	},
	"v1.30": {
		"kubernetes":     "v1.30.3",
		"etcd":           "3.5.12-0",
		"containerd":     "1.7.20",
		"runc":           "1.1.13",
		"pause":          "3.9",
		"coredns":        "1.11.1",
		"metrics-server": "0.7.1",
	},
}

func GetVersion(v string) map[string]string {
	return List[v]
}

type Release struct {
	TagName string `json:"tag_name"`
}

func FetchReleases() ([]Release, error) {
	var allReleases []Release
	page := 1

	for {
		url := fmt.Sprintf("https://api.github.com/repos/kubernetes/kubernetes/releases?page=%d", page)
		client := &http.Client{Timeout: 10 * time.Second}
		resp, err := client.Get(url)
		if err != nil {
			return nil, err
		}
		defer resp.Body.Close()

		var releases []Release
		if err := json.NewDecoder(resp.Body).Decode(&releases); err != nil {
			return nil, err
		}

		if len(releases) == 0 {
			break
		}

		allReleases = append(allReleases, releases...)
		page++
	}

	return allReleases, nil
}

func Version() {
	releases, err := FetchReleases()
	if err != nil {
		fmt.Println("Error fetching releases:", err)
		return
	}

	for _, release := range releases {
		if isStableRelease(release.TagName) && isVersionAtLeast(release.TagName, "v1.14.0") {
			fmt.Println(release.TagName)
		}
	}
}

func isStableRelease(tag string) bool {
	tag = strings.ToLower(tag)
	return !strings.Contains(tag, "alpha") && !strings.Contains(tag, "beta") && !strings.Contains(tag, "rc")
}

// isVersionAtLeast checks if a version is at least the given minimum version
func isVersionAtLeast(version, minVersion string) bool {
	v1 := parseVersion(version)
	v2 := parseVersion(minVersion)

	for i := 0; i < len(v1); i++ {
		if v1[i] > v2[i] {
			return true
		} else if v1[i] < v2[i] {
			return false
		}
	}
	return true
}

// parseVersion parses a version string into a slice of integers
func parseVersion(version string) []int {
	version = strings.TrimPrefix(version, "v")
	parts := strings.Split(version, ".")
	parsed := make([]int, len(parts))

	for i, part := range parts {
		parsed[i], _ = strconv.Atoi(part)
	}

	return parsed
}
