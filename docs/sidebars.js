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
      label: 'Application Deployment',
      collapsed: false,
      items:[
        'application',
        "rollout/rollout",
        'rollout/appdeploy',
        {
          'More Operations': [
            'end-user/kubectlplugin',
            'end-user/explore',
            'end-user/diagnose',
            'end-user/expose',
            'end-user/scale',
            'end-user/labels',
            'end-user/sidecar',
            'end-user/cloud-resources',
            'end-user/volumes',
            'end-user/monitoring',
            'end-user/health',
          ]
        },
      ]
    },
    {
      type: 'category',
      label: 'Platform Operation Guide',
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
              type: 'category',
              label: 'Defining Cloud Service',
              items: [
                'platform-engineers/cloud-services'
              ]
            },
          ]
        },
        {
          type: 'category',
          label: 'Defining Traits',
          items: [
            'cue/trait',
            'cue/patch-trait',
            'cue/status',
            'cue/advanced',
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
        {
          type: 'category',
          label: 'Capabilities',
          items: [
            'developers/references/README',
            'developers/references/component-types/webservice',
            'developers/references/component-types/task',
            'developers/references/component-types/worker',
            'developers/references/traits/route',
            'developers/references/traits/metrics',
            'developers/references/traits/scaler',
            'developers/references/restful-api/rest',
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
