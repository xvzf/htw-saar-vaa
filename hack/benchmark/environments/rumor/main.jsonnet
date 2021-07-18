function(scenario) {
  apiVersion: 'tanka.dev/v1alpha1',
  kind: 'Environment',
  metadata: {
    name: 'default',
  },
  spec: {
    apiServer: 'https://kubernetes.docker.internal:6443',
    namespace: 'default',
  },
  data:
    {
      local this = self,
      _config:: {
        graphs: (import 'assets/main.jsonnet'),

        // Determine scenario info
        _ss:: std.split(scenario, '-'),
        n: std.parseInt(self._ss[0]),
        m: std.parseInt(self._ss[1]),
        c: std.parseInt(self._ss[2]),

      },
      local k = (import 'ksonnet-util/kausal.libsonnet'),

      configmap: k.core.v1.configMap.new('config')
                 + k.core.v1.configMap.withDataMixin({
                   'graph.txt': this._config.graphs['%(n)s-%(m)s' % this._config],
                   'nodes.txt': std.join('\n', [
                     '%(n)s node-%(scenario)s-%(n)s:4000' % { n: n, scenario: scenario }
                     for n in std.range(1, this._config.n)
                   ]),
                 }),

      container:: k.core.v1.container.new('node', 'vaa-node:latest')
                  + k.core.v1.container.withImagePullPolicy('Never'),

      pods: [
        local config = {
          nuid: i,
          scenario: scenario,
        };

        local container = this.container
                          + k.core.v1.container.withVolumeMountsMixin([{ name: 'config', mountPath: '/config' }])
                          + k.core.v1.container.withPortsMixin([{ name: 'tcp', containerPort: 4000 }])
                          + k.core.v1.container.withArgs(
                            [
                              '--uid=%(nuid)d' % config,
                              '--config=/config/nodes.txt',
                              '--graph=/config/graph.txt',
                              '--graph=/config/graph.txt',
                            ]
                          );

        k.core.v1.pod.new('node-%(scenario)s-%(nuid)d' % config)
        + k.core.v1.pod.metadata.withLabelsMixin({
          n: std.toString(this._config.n),
          m: std.toString(this._config.m),
          c: std.toString(this._config.c),
          node: 'node-%(scenario)s-%(nuid)d' % config,
        })
        + k.core.v1.pod.spec.withContainersMixin([container])
        + k.core.v1.pod.spec.withVolumesMixin([{ name: 'config', configMap: { name: this.configmap.metadata.name } }])
        + k.core.v1.pod.spec.withNodeName('docker-desktop')  // remove scheduler load for faster execution

        for i in std.range(1, this._config.n)
      ],

      svs: [
        k.core.v1.service.new(
          name=pod.metadata.name,
          selector={ node: pod.metadata.labels.node },
          ports=[{ port: 4000, targetPort: 4000 }]
        )
        for pod in this.pods
      ],
    },
}
