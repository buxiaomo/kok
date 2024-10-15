create table version
(
    id                 INTEGER
        primary key autoincrement,
    kubernetes         TEXT
        constraint uni_version_kubernetes
            unique,
    etcd               TEXT,
    containerd         TEXT,
    runc               TEXT,
    pause              TEXT,
    coredns            TEXT,
    metrics_server     TEXT,
    kube_state_metrics TEXT
);

INSERT INTO version (id, kubernetes, etcd, containerd, runc, pause, coredns, metrics_server, kube_state_metrics) VALUES (1, 'v1.14.10', '3.3.10-0', '1.5.13', '1.1.2', '3.1', '1.3.1', '0.5.2', '1.9.8');
INSERT INTO version (id, kubernetes, etcd, containerd, runc, pause, coredns, metrics_server, kube_state_metrics) VALUES (2, 'v1.15.12', '3.3.10-0', '1.5.13', '1.1.2', '3.1', '1.3.1', '0.5.2', '1.9.8');
INSERT INTO version (id, kubernetes, etcd, containerd, runc, pause, coredns, metrics_server, kube_state_metrics) VALUES (3, 'v1.16.15', '3.3.10-0', '1.5.13', '1.1.2', '3.1', '1.6.2', '0.5.2', '1.9.8');
INSERT INTO version (id, kubernetes, etcd, containerd, runc, pause, coredns, metrics_server, kube_state_metrics) VALUES (4, 'v1.17.17', '3.4.3-0', '1.5.13', '1.1.2', '3.1', '1.6.5', '0.5.2', '2.0.0');
INSERT INTO version (id, kubernetes, etcd, containerd, runc, pause, coredns, metrics_server, kube_state_metrics) VALUES (5, 'v1.18.20', '3.4.3-0', '1.5.13', '1.1.2', '3.2', '1.6.7', '0.5.2', '2.0.0');
INSERT INTO version (id, kubernetes, etcd, containerd, runc, pause, coredns, metrics_server, kube_state_metrics) VALUES (6, 'v1.19.16', '3.4.13-0', '1.5.13', '1.1.2', '3.2', '1.7.0', '0.5.2', '2.2.4');
INSERT INTO version (id, kubernetes, etcd, containerd, runc, pause, coredns, metrics_server, kube_state_metrics) VALUES (7, 'v1.20.15', '3.4.13-0', '1.5.13', '1.1.2', '3.2', '1.7.0', '0.6.1', '2.2.4');
INSERT INTO version (id, kubernetes, etcd, containerd, runc, pause, coredns, metrics_server, kube_state_metrics) VALUES (8, 'v1.21.14', '3.4.13-0', '1.5.13', '1.1.2', '3.4.1', '1.8.0', '0.6.1', '2.4.2');
INSERT INTO version (id, kubernetes, etcd, containerd, runc, pause, coredns, metrics_server, kube_state_metrics) VALUES (9, 'v1.22.17', '3.5.0-0', '1.5.13', '1.1.2', '3.5', '1.8.4', '0.6.2', '2.6.0');
INSERT INTO version (id, kubernetes, etcd, containerd, runc, pause, coredns, metrics_server, kube_state_metrics) VALUES (10, 'v1.23.17', '3.5.1-0', '1.7.0', '1.1.4', '3.6', '1.8.6', '0.6.3', '2.6.0');
INSERT INTO version (id, kubernetes, etcd, containerd, runc, pause, coredns, metrics_server, kube_state_metrics) VALUES (11, 'v1.24.17', '3.5.3-0', '1.7.6', '1.1.9', '3.6', '1.8.6', '0.6.4', '2.6.0');
INSERT INTO version (id, kubernetes, etcd, containerd, runc, pause, coredns, metrics_server, kube_state_metrics) VALUES (12, 'v1.25.16', '3.5.4-0', '1.7.13', '1.1.12', '3.8', '1.9.3', '0.6.4', '2.7.0');
INSERT INTO version (id, kubernetes, etcd, containerd, runc, pause, coredns, metrics_server, kube_state_metrics) VALUES (13, 'v1.26.15', '3.5.6-0', '1.7.18', '1.1.12', '3.9', '1.9.3', '0.7.2', '0.7.1');
INSERT INTO version (id, kubernetes, etcd, containerd, runc, pause, coredns, metrics_server, kube_state_metrics) VALUES (14, 'v1.27.16', '3.5.7-0', '1.7.23', '1.1.14', '3.9', '1.10.1', '0.7.2', '2.10.1');
INSERT INTO version (id, kubernetes, etcd, containerd, runc, pause, coredns, metrics_server, kube_state_metrics) VALUES (15, 'v1.28.14', '3.5.9-0', '1.7.23', '1.1.14', '3.9', '1.10.1', '0.7.2', '2.11.0');
INSERT INTO version (id, kubernetes, etcd, containerd, runc, pause, coredns, metrics_server, kube_state_metrics) VALUES (16, 'v1.29.9', '3.5.12-0', '1.7.23', '1.1.14', '3.9', '1.11.1', '0.7.2', '2.12.0');
INSERT INTO version (id, kubernetes, etcd, containerd, runc, pause, coredns, metrics_server, kube_state_metrics) VALUES (17, 'v1.30.5', '3.5.12-0', '1.7.23', '1.1.14', '3.9', '1.11.1', '0.7.2', '2.13.0');
INSERT INTO version (id, kubernetes, etcd, containerd, runc, pause, coredns, metrics_server, kube_state_metrics) VALUES (18, 'v1.31.1', '3.5.15-0', '1.7.23', '1.1.14', '3.10', '1.11.1', '0.7.2', '2.13.0');
