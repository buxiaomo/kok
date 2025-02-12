FROM golang:1.23.4-alpine3.21 AS builder
#ENV GOPROXY "https://goproxy.cn,direct"
RUN apk add --no-cache g++ git curl bash openssl binutils-gold \
    && curl -fsSL https://raw.githubusercontent.com/helm/helm/main/scripts/get-helm-3 | bash
WORKDIR /go/src/app
COPY go.mod go.sum /go/src/app/
RUN go mod download
COPY . /go/src/app/
RUN CGO_ENABLED=1 GO111MODULE=on GOOS=linux go build -o main main.go

FROM alpine:3.21.2
RUN apk --no-cache upgrade \
    && apk add --no-cache curl bash sqlite bash-completion git \
    && adduser -D -h /app -u 1000 app
WORKDIR /app
ARG KUBE_VERSION=v1.32.1
RUN if [ `uname -m` = "x86_64" ]; then \
        wget -q https://dl.k8s.io/${KUBE_VERSION}/bin/linux/amd64/kubectl -O /usr/local/bin/kubectl;  \
    else \
        wget -q https://dl.k8s.io/${KUBE_VERSION}/bin/linux/arm64/kubectl -O /usr/local/bin/kubectl; \
    fi \
    && echo 'source <(kubectl completion bash)' > /etc/profile.d/kubelet.sh \
    && chmod +x /usr/local/bin/kubectl

ARG KUBECM_VERSION=v0.31.0
RUN if [ `uname -m` = "x86_64" ]; then \
        wget https://github.com/sunny0826/kubecm/releases/download/${KUBECM_VERSION}/kubecm_${KUBECM_VERSION}_Linux_x86_64.tar.gz -O /tmp/kubecm.tar.gz;  \
    else \
        wget https://github.com/sunny0826/kubecm/releases/download/${KUBECM_VERSION}/kubecm_${KUBECM_VERSION}_Linux_arm64.tar.gz -O /tmp/kubecm.tar.gz;  \
    fi \
    && tar -zvxf /tmp/kubecm.tar.gz -C /usr/local/bin/ kubecm \
    && echo 'source <(kubecm completion bash)' > /etc/profile.d/kubecm.sh \
    && chmod +x /usr/local/bin/kubecm \
    && rm -rf /tmp/kubecm.tar.gz

COPY --from=builder --chown=1000 /go/src/app/main ./main
COPY --from=builder --chown=1000 /go/src/app/templates ./templates
#COPY --from=builder --chown=1000 /go/src/app/appmarket ./appmarket
COPY --from=builder --chown=1000 /go/src/app/static ./static
COPY --from=builder --chown=1000 /usr/local/bin/helm /usr/local/bin/helm
COPY entrypoint.sh /entrypoint.sh
VOLUME /app/data
EXPOSE 8080
USER 1000
CMD ["/entrypoint.sh"]
