module.exports = {
  docs: [
    {
      type: 'category',
      label: 'Overview',
      collapsed: false,
      items: [
        'introduction',
      ],
    },
    {
      type: 'category',
      label: 'Getting Started',
      collapsed: false,
      items: [
        'install',
        'quick-start',
        'concepts',
      ],
    },
    {
      type: 'category',
      label: 'Application Team Guide',
      collapsed: false,
      items:[
        'end-user/application',
        {
          'Components': [
            'end-user/components/webservice',
            'end-user/components/task',
            'end-user/components/worker',
            'end-user/components/cloud-services',
            'end-user/components/more',
          ]
        },
        {
          'Traits': [
            'end-user/traits/ingress',
            'end-user/traits/scaler',
            'end-user/traits/annotations-and-labels',
            'end-user/traits/sidecar',
            'end-user/traits/volumes',
            'end-user/traits/service-binding',
            'end-user/traits/more',
          ]
        },
        'end-user/scopes/rollout-plan',
        {
          'Observability': [
            'end-user/scopes/health',
          ]
        },
        {
          'Debugging': [
            'end-user/debug',
          ]
        },
      ]
    },
    {
      type: 'category',
      label: 'Platform Team Guide',
      collapsed: false,
      items: [
        'platform-engineers/overview',
        'platform-engineers/definition-and-templates',
        'platform-engineers/openapi-v3-json-schema',
        {
          type: 'category',
          label: 'Defining Components',
          items: [
            {
              'CUE': [
                'platform-engineers/cue/component',
                'platform-engineers/cue/basic',
              ]
            },
            {
              'Helm': [
                  'platform-engineers/helm/component',
                  'platform-engineers/helm/trait',
                  'platform-engineers/helm/known-issues'
              ]
            },
            {
              'Simple Template': [
                  'platform-engineers/kube/component',
                  'platform-engineers/kube/trait',
              ]
            },
            {
              type: 'category',
              label: 'Cloud Services',
              items: [
                'platform-engineers/cloud-services',
                'platform-engineers/terraform',
                'platform-engineers/crossplane',
              ]
            },
          ]
        },
        {
          type: 'category',
          label: 'Defining Traits',
          items: [
            'platform-engineers/cue/trait',
            'platform-engineers/cue/patch-trait',
            'platform-engineers/cue/status',
            'platform-engineers/cue/advanced',
          ]
        },
        {
          type: 'category',
          label: 'Hands-on Lab',
          items: [
            'platform-engineers/debug-test-cue',
            'platform-engineers/keda'
          ]
        },
      ],
    },
    {
      type: 'category',
      label: 'Using KubeVela CLI',
      items: [
        {
          'Appfile': [
            'quick-start-appfile',
            'developers/learn-appfile',
          ]
        },
        {
          'Managing Applications': [
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
      label: 'References',
      items: [
        {
          type: 'category',
          label: 'CLI',
          items: [
            'cli/vela_components',
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
      'Appendix': [
        'advanced-install',
      ]
    },
    {
      type: 'doc',
      id: 'developers/references/devex/faq'
    },
  ],
};
