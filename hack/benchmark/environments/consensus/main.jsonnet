{
  environment(scenario):: {
    apiVersion: 'tanka.dev/v1alpha1',
    kind: 'Environment',
    metadata: {
      name: scenario.name,
    },
    spec: {
      apiServer: 'https://35.193.107.208',
      namespace: 'default',
    },
    data:
      {
        local this = self,
        _config:: {
          graphs: (import 'assets/main.jsonnet'),

          // Determine scenario info
          n: scenario.n,
          m: scenario.e,
        },
        local k = (import 'ksonnet-util/kausal.libsonnet'),

        configmap: k.core.v1.configMap.new('config-%(name)s' % scenario)
                   + k.core.v1.configMap.withDataMixin({
                     'graph.txt': this._config.graphs['%(n)s-%(m)s' % this._config],
                     'nodes.txt': std.join('\n', [
                       '%(n)s node-%(name)s-%(scenario)s-%(n)s:4000' % { n: n, scenario: scenario.scenario, name: scenario.name }
                       for n in std.range(1, this._config.n)
                     ]),
                   }),

        container:: k.core.v1.container.new('node', 'gcr.io/playground-s-11-f14ea1c1/vaa:v3')
                    + k.core.v1.container.withImagePullPolicy('IfNotPresent'),

        pods: [
          local config = scenario {
            nuid: i,
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
                                '--consensus-m=%(m)d' % config,
                                '--consensus-amax=%(amax)d' % config,
                                '--consensus-p=%(p)d' % config,
                                '--consensus-s=%(s)d' % config,
                              ]
                            );

          k.core.v1.pod.new('node-%(name)s-%(scenario)s-%(nuid)d' % config)
          + k.core.v1.pod.metadata.withLabelsMixin({
            n: std.toString(this._config.n),
            e: std.toString(this._config.m),
            scenario: scenario.name,
            consensusm: std.toString(scenario.m),
            consensusamax: std.toString(scenario.amax),
            consensusp: std.toString(scenario.p),
            consensuss: std.toString(scenario.s),
            node: 'node-%(name)s-%(scenario)s-%(nuid)d' % config,
          })
          + k.core.v1.pod.spec.withContainersMixin([container])
          + k.core.v1.pod.spec.withVolumesMixin([{ name: 'config', configMap: { name: this.configmap.metadata.name } }])

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
  },

  scenarios:: [
    /*
    { scenario: '10-15', name: 'test10', e: 15, n: 10, m: 5, amax: 5, p: 2, s: 1 },
    { scenario: '6-9', name: 'test0', e: 9, n: 6, m: 5, amax: 10, p: 3, s: 1 },
    { scenario: '6-9', name: 'test1', e: 9, n: 6, m: 4, amax: 20, p: 2, s: 3 },
    { scenario: '6-9', name: 'test2', e: 9, n: 6, m: 3, amax: 5, p: 2, s: 3 },
    { scenario: '6-9', name: 'test3', e: 9, n: 6, m: 6, amax: 15, p: 4, s: 2 },
    { scenario: '6-9', name: 'test4', e: 9, n: 6, m: 8, amax: 20, p: 4, s: 3 },
    { scenario: '8-12', name: 'test5', e: 12, n: 8, m: 5, amax: 10, p: 2, s: 2 },
    { scenario: '8-12', name: 'test6', e: 12, n: 8, m: 4, amax: 15, p: 1, s: 5 },
    { scenario: '8-12', name: 'test7', e: 12, n: 8, m: 6, amax: 20, p: 2, s: 4 },
    { scenario: '8-12', name: 'test8', e: 12, n: 8, m: 6, amax: 25, p: 1, s: 1 },
    { scenario: '8-12', name: 'test9', e: 12, n: 8, m: 6, amax: 5, p: 3, s: 3 },
    */
    { scenario: '6-9', name: 'test' + i, e: 9, n: 6, m: 5, amax: 10, p: 2, s: 2 }
    for i in std.range(0, 10)
  ],

  envs: {
    [scenario.name]: $.environment(scenario)
    for scenario in $.scenarios
  },

}
