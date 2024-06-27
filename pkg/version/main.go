package version

var List = map[string]map[string]string{
	"v1.14": {
		"kubernetes": "v1.14.10",
		"etcd":       "3.3.10-0",
		"containerd": "1.5.13",
		"runc":       "1.1.2",
	},
	"v1.15": {
		"kubernetes": "v1.15.12",
		"etcd":       "3.3.10-0",
		"containerd": "1.5.13",
		"runc":       "1.1.2",
	},
	"v1.16": {
		"kubernetes": "v1.16.15",
		"etcd":       "3.3.10-0",
		"containerd": "1.5.13",
		"runc":       "1.1.2",
	},
	"v1.17": {
		"kubernetes": "v1.17.17",
		"etcd":       "3.4.3-0",
		"containerd": "1.5.13",
		"runc":       "1.1.2",
	},
	"v1.18": {
		"kubernetes": "v1.18.20",
		"etcd":       "3.4.3-0",
		"containerd": "1.5.13",
		"runc":       "1.1.2",
	},
	"v1.19": {
		"kubernetes": "v1.19.16",
		"etcd":       "3.4.13-0",
		"containerd": "1.5.13",
		"runc":       "1.1.2",
	},
	"v1.20": {
		"kubernetes": "v1.20.15",
		"etcd":       "3.4.13-0",
		"containerd": "1.5.13",
		"runc":       "1.1.2",
	},
	"v1.21": {
		"kubernetes": "v1.21.14",
		"etcd":       "3.4.13-0",
		"containerd": "1.5.13",
		"runc":       "1.1.2",
	},
	"v1.22": {
		"kubernetes": "v1.22.17",
		"etcd":       "3.5.0-0",
		"containerd": "1.5.13",
		"runc":       "1.1.2",
	},
	"v1.23": {
		"kubernetes": "v1.23.17",
		"etcd":       "3.5.1-0",
		"containerd": "1.7.0",
		"runc":       "1.1.4",
	},
	"v1.24": {
		"kubernetes": "v1.24.17",
		"etcd":       "3.5.3-0",
		"containerd": "1.7.6",
		"runc":       "1.1.9",
	},
	"v1.25": {
		"kubernetes": "v1.25.16",
		"etcd":       "3.5.4-0",
		"containerd": "1.7.13",
		"runc":       "1.1.12",
	},
	"v1.26": {
		"kubernetes": "v1.26.15",
		"etcd":       "3.5.6-0",
		"containerd": "1.7.18",
		"runc":       "1.1.12",
	},
	"v1.27": {
		"kubernetes": "v1.27.15",
		"etcd":       "3.5.7-0",
		"containerd": "1.7.18",
		"runc":       "1.1.12",
	},
	"v1.28": {
		"kubernetes": "v1.28.11",
		"etcd":       "3.5.9-0",
		"containerd": "1.7.18",
		"runc":       "1.1.12",
	},
	"v1.29": {
		"kubernetes": "v1.29.6",
		"etcd":       "3.5.12-0",
		"containerd": "1.7.18",
		"runc":       "1.1.12",
	},
	"v1.30": {
		"kubernetes": "v1.30.2",
		"etcd":       "3.5.12-0",
		"containerd": "1.7.18",
		"runc":       "1.1.12",
	},
}

func GetVersion(v string) map[string]string {
	return List[v]
}
