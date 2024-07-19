# appmarket

1. Package tgz
```shell
# in `charts/<application>` dir
helm package --destination ../../assets <version>

or
ls  | xargs helm package --destination ../../assets
```

2. Generate index.yaml
```shell
# in `appmarket` dir
helm repo index ./assets/
```


```
{{- define "pdbApiVersion" -}}
{{- if .Capabilities.APIVersions.Has "policy/v1/PodDisruptionBudget" -}}
policy/v1
{{- else -}}
policy/v1beta1
{{- end -}}
{{- end -}}
apiVersion: {{ include "pdbApiVersion" . }}
```