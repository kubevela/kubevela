module.exports = {
  docs: [
    {
      type: 'category',
      label: 'Overview',
      items: [
        'introduction',
        'concepts',
      ],
    },
    {
      type: 'category',
      label: 'Getting Started',
      items: [
        'install',
        'quick-start',
      ],
    },
    {
      type: 'category',
      label: 'Platform Builder Guide',
      items: [
        {
          'Design Abstraction': [
            'platform-engineers/overview',
            'application',
            'platform-engineers/definition-and-templates',
          ]
        },
        {
          'Visualization': [
            'platform-engineers/openapi-v3-json-schema'
          ]
        },
        {
          type: 'category',
          label: 'Defining Components',
          items: [
            {
              'CUE': [
                'cue/component',
                'cue/basic',
              ]
            },
            {
              'Helm': [
                  'helm/component',
                  'helm/trait',
                  'helm/known-issues'
              ]
            },
            {
              'Raw Template': [
                  'kube/component',
                  'kube/trait',
              ]
            },
            {
              'Defining Traits': [
                  'cue/trait',
                  'cue/patch-trait',
                  'cue/status',
                  'cue/advanced',
              ]
            },
            {
              'Hands-on Lab': [
                  'platform-engineers/debug-test-cue',
                  'platform-engineers/keda'
              ]
            }
          ]
        },
      ],
    },
    {
      type: 'category',
      label: 'Developer Experience Guide',
      items: [
        {
          'Appfile': [
            'quick-start-appfile',
            'developers/learn-appfile',
          ]
        },
        {
          'Command Line Tool (CLI)': [
            'developers/config-enviroments',
            'developers/port-forward',
            'developers/check-logs',
            'developers/exec-cmd',
            'developers/cap-center',
            'developers/config-app',
          ]
        },
      ],
    },
    {
      type: 'category',
      label: 'CLI References',
      items: [
        'cli/vela_config',
        'cli/vela_env',
        'cli/vela_init',
        'cli/vela_up',
        'cli/vela_version',
        'cli/vela_exec',
        'cli/vela_logs',
        'cli/vela_ls',
        'cli/vela_port-forward',
        'cli/vela_show',
        'cli/vela_status',
        'cli/vela_workloads',
        'cli/vela_traits',
        'cli/vela_system',
        'cli/vela_template',
        'cli/vela_cap',
      ],
    },
    {
      type: 'category',
      label: 'Capability References',
      items: [
        'developers/references/README',
        'developers/references/workload-types/webservice',
        'developers/references/workload-types/task',
        'developers/references/workload-types/worker',
        'developers/references/traits/route',
        'developers/references/traits/metrics',
        'developers/references/traits/scaler',
        'developers/references/restful-api/rest',
      ],
    },
    {
      type: 'category',
      label: 'Roadmap',
      items: [
        'roadmap',
      ],
    },
    {
      type: 'doc',
      id: 'developers/references/devex/faq'
    },
  ],
};
