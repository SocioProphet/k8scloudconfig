package template

const WorkerTemplate = `---
ignition:
  version: "2.2.0"
passwd:
  users:
    - name: giantswarm
      shell: "/bin/bash"
      uid: 1000
      groups:
        - "sudo"
        - "docker"
      sshAuthorizedKeys:
        - "ssh-rsa AAAAB3NzaC1yc2EAAAADAQABAAACAQCuJvxy3FKGrfJ4XB5exEdKXiqqteXEPFzPtex6dC0lHyigtO7l+NXXbs9Lga2+Ifs0Tza92MRhg/FJ+6za3oULFo7+gDyt86DIkZkMFdnSv9+YxYe+g4zqakSV+bLVf2KP6krUGJb7t4Nb+gGH62AiUx+58Onxn5rvYC0/AXOYhkAiH8PydXTDJDPhSA/qWSWEeCQistpZEDFnaVi0e7uq/k3hWJ+v9Gz0qqChHKWWOYp3W6aiIE3G6gLOXNEBdWRrjK6xmrSmo9Toqh1G7iIV0Y6o9w5gIHJxf6+8X70DCuVDx9OLHmjjMyGnd+1c3yTFMUdugtvmeiGWE0E7ZjNSNIqWlnvYJ0E1XPBiyQ7nhitOtVvPC4kpRP7nOFiCK9n8Lr3z3p4v3GO0FU3/qvLX+ECOrYK316gtwSJMd+HIouCbaJaFGvT34peaq1uluOP/JE+rFOnszZFpCYgTY2b4lWjf2krkI/a/3NDJPnRpjoE3RjmbepkZeIdOKTCTH1xYZ3O8dWKRX8X4xORvKJO+oV2UdoZlFa/WJTmq23z4pCVm0UWDYR5C2b9fHwxh/xrPT7CQ0E+E9wmeOvR4wppDMseGQCL+rSzy2AYiQ3D8iQxk0r6T+9MyiRCfuY73p63gB3m37jMQSLHvm77MkRnYcBy61Qxk+y+ls2D0xJfqxw== giantswarm"
{{ range $index, $user := .Cluster.Kubernetes.SSH.UserList }}
    - name: {{ $user.Name }}
      shell: "/bin/bash"
      groups:
        - "sudo"
        - "docker"
{{ if ne $user.PublicKey "" }}
      sshAuthorizedKeys:
        - "{{ $user.PublicKey }}"
{{ end }}
{{ end }}

systemd:
  units:
  # Start - manual management for cgroup structure
  - name: kubereserved.slice
    path: /etc/systemd/system/kubereserved.slice
    content: |
      [Unit]
      Description=Limited resources slice for Kubernetes services
      Documentation=man:systemd.special(7)
      DefaultDependencies=no
      Before=slices.target
      Requires=-.slice
      After=-.slice
  # End - manual management for cgroup structure
  {{range .Extension.Units}}
  - name: {{.Metadata.Name}}
    enabled: {{.Metadata.Enabled}}
    contents: |
      {{range .Content}}{{.}}
      {{end}}{{end}}
  {{ range .KVMWorkerMountTags }}
  - name: data-{{ . }}.mount
    enabled: true
    contents: |
      Description=Guest mount for {{ . }} host volume
      [Mount]
      What={{ . }}
      Where=/data/{{ . }}
      Options=trans=virtio,version=9p2000.L,cache=mmap
      Type=9p
      [Install]
      WantedBy=multi-user.target
  {{ end }}
  - name: set-certs-group-owner-permission-giantswarm.service
    enabled: true
    contents: |
      [Unit]
      Description=Change group owner for certificates to giantswarm
      Wants=k8s-kubelet.service k8s-setup-network-env.service
      After=k8s-kubelet.service k8s-setup-network-env.service
      [Service]
      Type=oneshot
      ExecStart=/bin/sh -c "find /etc/kubernetes/ssl -type f -print | xargs -i  sh -c 'chown root:giantswarm {} && chmod 640 {}'"
      [Install]
      WantedBy=multi-user.target
  - name: wait-for-domains.service
    enabled: true
    contents: |
      [Unit]
      Description=Wait for etcd and k8s API domains to be available
      [Service]
      Type=oneshot
      ExecStart=/opt/wait-for-domains
      [Install]
      WantedBy=multi-user.target
  - name: os-hardening.service
    enabled: true
    contents: |
      [Unit]
      Description=Apply os hardening
      [Service]
      Type=oneshot
      ExecStartPre=-/bin/bash -c "gpasswd -d core rkt; gpasswd -d core docker; gpasswd -d core wheel"
      ExecStartPre=/bin/bash -c "until [ -f '/etc/sysctl.d/hardening.conf' ]; do echo Waiting for sysctl file; sleep 1s;done;"
      ExecStart=/usr/sbin/sysctl -p /etc/sysctl.d/hardening.conf
      [Install]
      WantedBy=multi-user.target
  - name: k8s-setup-kubelet-environment.service
    enabled: true
    contents: |
      [Unit]
      Description=k8s-setup-kubelet-environment Service
      After=k8s-setup-network-env.service docker.service
      Requires=k8s-setup-network-env.service docker.service
      [Service]
      Type=oneshot
      RemainAfterExit=yes
      TimeoutStartSec=0
      ExecStart=/opt/bin/setup-kubelet-environment worker
      [Install]
      WantedBy=multi-user.target
  - name: k8s-setup-kubelet-config.service
    enabled: true
    contents: |
      [Unit]
      Description=k8s-setup-kubelet-config Service
      After=k8s-setup-network-env.service docker.service k8s-setup-kubelet-environment.service
      Requires=k8s-setup-network-env.service docker.service k8s-setup-kubelet-environment.service
      [Service]
      Type=oneshot
      RemainAfterExit=yes
      TimeoutStartSec=0
      EnvironmentFile=/etc/network-environment
      EnvironmentFile=/etc/kubelet-environment
      ExecStart=/bin/bash -c '/usr/bin/envsubst </etc/kubernetes/config/kubelet.yaml.tmpl >/etc/kubernetes/config/kubelet.yaml'
      [Install]
      WantedBy=multi-user.target
  - name: containerd.service
    enabled: true
    contents: |
    dropins:
      - name: 10-change-cgroup.conf
        contents: |
          [Service]
          CPUAccounting=true
          MemoryAccounting=true
          Slice=kubereserved.slice
  - name: docker.service
    enabled: true
    contents: |
    dropins:
      - name: 10-giantswarm-extra-args.conf
        contents: |
          [Service]
          CPUAccounting=true
          MemoryAccounting=true
          Slice=kubereserved.slice
          Environment="DOCKER_CGROUPS=--exec-opt native.cgroupdriver=cgroupfs --cgroup-parent=/kubereserved.slice --log-opt max-size=25m --log-opt max-file=2 --log-opt labels=io.kubernetes.container.hash,io.kubernetes.container.name,io.kubernetes.pod.name,io.kubernetes.pod.namespace,io.kubernetes.pod.uid"
          Environment="DOCKER_OPT_BIP=--bip={{.Cluster.Docker.Daemon.CIDR}}"
          {{- if .Proxy.HTTP }}
          Environment="HTTP_PROXY={{ .Proxy.HTTP }}"
          {{- end }}
          {{- if .Proxy.HTTPS }}
          Environment="HTTPS_PROXY={{ .Proxy.HTTPS }}"
          {{- end }}
          {{- if .Proxy.NoProxy }}
          Environment="NO_PROXY={{ .Proxy.NoProxy }}"
          {{- end }}
  - name: k8s-setup-network-env.service
    enabled: true
    contents: |
      [Unit]
      Description=k8s-setup-network-env Service
      Wants=network.target docker.service wait-for-domains.service
      After=network.target docker.service wait-for-domains.service
      [Service]
      Type=oneshot
      TimeoutStartSec=0
      Environment="IMAGE={{ .Images.KubernetesNetworkSetupDocker }}"
      Environment="NAME=%p.service"
      ExecStartPre=/usr/bin/mkdir -p /opt/bin/
      ExecStartPre=/usr/bin/docker pull $IMAGE
      ExecStartPre=-/usr/bin/docker stop -t 10 $NAME
      ExecStartPre=-/usr/bin/docker rm -f $NAME
      ExecStart=/usr/bin/docker run --rm --net=host -v /etc:/etc --name $NAME $IMAGE
      ExecStop=-/usr/bin/docker stop -t 10 $NAME
      ExecStopPost=-/usr/bin/docker rm -f $NAME
      [Install]
      WantedBy=multi-user.target
  - name: k8s-extract.service
    enabled: true
    contents: |
      [Unit]
      Description=k8s-extract Service
      After=docker.service
      Requires=docker.service
      [Service]
      Type=oneshot
      RemainAfterExit=yes
      TimeoutStartSec=0
      Environment=IMAGE={{ .Images.Hyperkube }}
      Environment=CONTAINER_NAME=%p.service
      ExecStartPre=/usr/bin/mkdir -p /opt/bin/
      ExecStartPre=/usr/bin/docker pull $IMAGE
      ExecStartPre=-/usr/bin/docker rm $CONTAINER_NAME
      ExecStartPre=-/usr/bin/docker create --name $CONTAINER_NAME $IMAGE /kubectl
      ExecStart=/opt/k8s-extract $CONTAINER_NAME
      ExecStopPost=-/usr/bin/docker rm $CONTAINER_NAME
      [Install]
      WantedBy=multi-user.target
  - name: k8s-kubelet.service
    enabled: true
    contents: |
      [Unit]
      Wants=k8s-setup-network-env.service k8s-setup-kubelet-config.service k8s-extract.service{{ if eq .Cluster.Kubernetes.CloudProvider "" }} rpc-statd.service{{ end }}
      After=k8s-setup-network-env.service k8s-setup-kubelet-config.service k8s-extract.service{{ if eq .Cluster.Kubernetes.CloudProvider "" }} rpc-statd.service{{ end }}
      Description=k8s-kubelet
      StartLimitIntervalSec=0
      [Service]
      User=root
      TimeoutStartSec=300
      Restart=always
      RestartSec=0
      TimeoutStopSec=10
      Slice=kubereserved.slice
      CPUAccounting=true
      MemoryAccounting=true
      Environment="ETCD_CA_CERT_FILE=/etc/kubernetes/ssl/calico/etcd-ca"
      Environment="ETCD_CERT_FILE=/etc/kubernetes/ssl/calico/etcd-cert"
      Environment="ETCD_KEY_FILE=/etc/kubernetes/ssl/calico/etcd-key"
      EnvironmentFile=/etc/network-environment
      ExecStart=/opt/bin/kubelet \
        {{ range .Kubernetes.Kubelet.CommandExtraArgs -}}
        {{ . }} \
        {{ end -}}
        --node-ip=${DEFAULT_IPV4} \
        --config=/etc/kubernetes/config/kubelet.yaml \
        --logtostderr=true \
        --cloud-provider={{.Cluster.Kubernetes.CloudProvider}} \
        --pod-infra-container-image={{ .Images.Pause }} \
        --image-pull-progress-deadline={{.ImagePullProgressDeadline}} \
        --network-plugin=cni \
        --register-node=true \
        --kubeconfig=/etc/kubernetes/kubeconfig/kubelet.yaml \
        --node-labels="node.kubernetes.io/worker,role=worker,ip=${DEFAULT_IPV4},{{.Cluster.Kubernetes.Kubelet.Labels}}" \
        --v=2
      [Install]
      WantedBy=multi-user.target
  - name: k8s-label-node.service
    enabled: true
    contents: |
      [Unit]
      Description=Adds labels to the node after kubelet startup
      After=k8s-kubelet.service
      Wants=k8s-kubelet.service
      [Service]
      Type=oneshot
      Environment="PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/opt/bin"
      Environment="KUBECONFIG=/etc/kubernetes/kubeconfig/kubelet.yaml"
      ExecStart=/bin/sh -c '\
        while [ "$(kubectl get nodes $(hostname | tr '[:upper:]' '[:lower:]')| wc -l)" -lt "1" ]; do echo "Waiting for healthy k8s" && sleep 20s;done; \
        kubectl label nodes --overwrite $(hostname | tr '[:upper:]' '[:lower:]') node-role.kubernetes.io/worker=""; \
        kubectl label nodes --overwrite $(hostname | tr '[:upper:]' '[:lower:]') kubernetes.io/role=worker; \
        for l in $(echo "{{.Cluster.Kubernetes.Kubelet.Labels}}" | tr "," " "); do \
            kubectl label nodes --overwrite $(hostname | tr "[:upper:]" "[:lower:]") $l; \
        done'
      [Install]
      WantedBy=multi-user.target
  - name: k8s-label-node.timer
    enabled: true
    contents: |
      [Unit]
      Description=Execute k8s-label-node every hour
      [Timer]
      OnCalendar=hourly
      [Install]
      WantedBy=multi-user.target
  - name: etcd2.service
    enabled: false
    mask: true
  - name: update-engine.service
    enabled: false
    mask: true
  - name: locksmithd.service
    enabled: false
    mask: true
  - name: fleet.service
    enabled: false
    mask: true
  - name: fleet.socket
    enabled: false
    mask: true
  - name: flanneld.service
    enabled: false
    mask: true
  - name: systemd-networkd-wait-online.service
    enabled: false
    mask: true

{{ if .Debug.Enabled }}
  - name: logentries.service
    enabled: true
    contents: |
      [Unit]
      Description=Logentries
      After=systemd-networkd.service
      Wants=systemd-networkd.service
      StartLimitBurst=10
      StartLimitIntervalSec=600

      [Service]
      Restart=on-failure
      RestartSec=5
      Environment=LOGENTRIES_PREFIX={{ .Debug.LogsPrefix }}-worker
      Environment=LOGENTRIES_TOKEN={{ .Debug.LogsToken }}
      ExecStart=/bin/sh -c 'journalctl -o short -f | sed \"s/^/${LOGENTRIES_TOKEN} ${LOGENTRIES_PREFIX} \\0/g\" | ncat data.logentries.com 10000'
      [Install]
      WantedBy=multi-user.target
{{ end }}

storage:
  directories:
    - path: /var/log/fluentbit_db
      filesystem: root
      mode: 2644
      user:
        name: giantswarm
      group:
        name: giantswarm
  files:
    - path: /boot/coreos/first_boot
      filesystem: root
    {{ if .ForceCGroupsV1 }}
    - path: /etc/flatcar-cgroupv1
      filesystem: root
      mode: 0444
    {{ end }}
    - path: /etc/ssh/trusted-user-ca-keys.pem
      filesystem: root
      mode: 0644
      contents:
        source: "data:text/plain;base64,{{ index .Files "conf/trusted-user-ca-keys.pem" }}"

    - path: /etc/kubernetes/config/kubelet.yaml.tmpl
      filesystem: root
      mode: 0644
      contents:
        source: "data:text/plain;charset=utf-8;base64,{{  index .Files "config/kubelet-worker.yaml.tmpl" }}"

    - path: /etc/kubernetes/kubeconfig/kubelet.yaml
      filesystem: root
      mode: 0644
      contents:
        source: "data:text/plain;charset=utf-8;base64,{{  index .Files "kubeconfig/kubelet-worker.yaml" }}"

    - path: /etc/kubernetes/config/proxy-config.yml
      filesystem: root
      mode: 0644
      contents:
        source: "data:text/plain;charset=utf-8;base64,{{  index .Files "config/kube-proxy.yaml" }}"

    - path: /etc/kubernetes/config/proxy-kubeconfig.yaml
      filesystem: root
      mode: 0644
      contents:
        source: "data:text/plain;charset=utf-8;base64,{{  index .Files "kubeconfig/kube-proxy-worker.yaml" }}"

    - path: /etc/kubernetes/kubeconfig/kube-proxy.yaml
      filesystem: root
      mode: 0644
      contents:
        source: "data:text/plain;charset=utf-8;base64,{{  index .Files "kubeconfig/kube-proxy-worker.yaml" }}"

    - path: /opt/wait-for-domains
      filesystem: root
      mode: 0544
      contents:
        source: "data:text/plain;charset=utf-8;base64,{{  index .Files "conf/wait-for-domains" }}"

    - path: /etc/ssh/sshd_config
      filesystem: root
      mode: 0644
      contents:
        source: "data:text/plain;charset=utf-8;base64,{{  index .Files "conf/sshd_config" }}"
    - path: /opt/bin/setup-kubelet-environment
      filesystem: root
      mode: 0544
      contents:
        source: "data:text/plain;charset=utf-8;base64,{{  index .Files "conf/setup-kubelet-environment" }}"

    - path: /etc/sysctl.d/hardening.conf
      filesystem: root
      mode: 0600
      contents:
        source: "data:text/plain;charset=utf-8;base64,{{  index .Files "conf/hardening.conf" }}"

    - path: /etc/audit/rules.d/10-docker.rules
      filesystem: root
      mode: 0600
      contents:
        source: "data:text/plain;charset=utf-8;base64,{{  index .Files "conf/10-docker.rules" }}"

    - path: /etc/docker/daemon.json
      filesystem: root
      mode: 0644
      contents:
        source: "data:text/plain;charset=utf-8;base64,{{  index .Files "conf/docker-daemon.json" }}"

    - path: /root/.docker/config.json
      filesystem: root
      mode: 0644
      contents:
        source: "data:text/plain;charset=utf-8;base64,{{  index .Files "conf/kubelet-docker-config.json" }}"

    - path: /etc/modules-load.d/ip_vs.conf
      filesystem: root
      mode: 0600
      contents:
        source: "data:text/plain;charset=utf-8;base64,{{  index .Files "conf/ip_vs.conf" }}"

    - path: /opt/k8s-extract
      filesystem: root
      mode: 0544
      contents:
        source: "data:text/plain;charset=utf-8;base64,{{  index .Files "conf/k8s-extract" }}"

    {{ range .Extension.Files -}}
    - path: {{ .Metadata.Path }}
      filesystem: root
      user:
      {{- if .Metadata.Owner.User.ID }}
        id: {{ .Metadata.Owner.User.ID }}
      {{- else }}
        name: {{ .Metadata.Owner.User.Name }}
      {{- end }}
      group:
      {{- if .Metadata.Owner.Group.ID }}
        id: {{ .Metadata.Owner.Group.ID }}
      {{- else }}
        name: {{ .Metadata.Owner.Group.Name }}
      {{- end }}
      mode: {{printf "%#o" .Metadata.Permissions}}
      contents:
        source: "data:text/plain;charset=utf-8;base64,{{ .Content }}"
        {{ if .Metadata.Compression }}
        compression: gzip
        {{end}}
    {{ end -}}

{{ range .Extension.VerbatimSections }}
{{ .Content }}
{{ end }}
`
