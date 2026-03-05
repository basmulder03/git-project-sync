import { defineConfig } from 'vitepress'

const isCI = process.env.GITHUB_ACTIONS === 'true'

export default defineConfig({
  title: 'Git Project Sync',
  description: 'Safe cross-platform Git repository synchronization',
  base: isCI ? '/git-project-sync/' : '/',
  cleanUrls: true,
  themeConfig: {
    nav: [
      { text: 'Getting Started', link: '/getting-started/installation-and-service-registration' },
      { text: 'Operations', link: '/operations/service-operations-guide' },
      { text: 'Reference', link: '/reference/cli-command-specification' },
      { text: 'Release', link: '/release/release-process-and-automation' }
    ],
    sidebar: [
      {
        text: 'Getting Started',
        items: [
          { text: 'Installation and Service Registration', link: '/getting-started/installation-and-service-registration' },
          { text: 'First-Run Onboarding', link: '/getting-started/first-run-onboarding' },
          { text: 'User Guide', link: '/user/day-to-day-usage-guide' },
          { text: 'Troubleshooting', link: '/support/common-failures-and-remediation' }
        ]
      },
      {
        text: 'Operations',
        items: [
          { text: 'Service Operations Guide', link: '/operations/service-operations-guide' },
          { text: 'Incident Response Playbook', link: '/operations/incident-response-playbook' },
          { text: 'Reliability SLOs and Error Budgets', link: '/operations/reliability-slos-and-error-budgets' }
        ]
      },
      {
        text: 'Reference',
        items: [
          { text: 'CLI Command Specification', link: '/reference/cli-command-specification' },
          { text: 'Configuration Schema', link: '/reference/configuration-schema' },
          { text: 'PAT Permission Requirements', link: '/reference/pat-permission-requirements' },
          { text: 'Security Model and Controls', link: '/security/security-model-and-controls' }
        ]
      },
      {
        text: 'Engineering',
        items: [
          { text: 'System Architecture', link: '/engineering/system-architecture' },
          { text: 'Test Strategy and Coverage', link: '/engineering/test-strategy-and-coverage' },
          { text: 'Acceptance Test Matrix', link: '/engineering/acceptance-test-matrix' }
        ]
      },
      {
        text: 'Release and Project',
        items: [
          { text: 'Release Process and Automation', link: '/release/release-process-and-automation' },
          { text: 'Release Candidate Checklist', link: '/release/release-candidate-checklist' },
          { text: 'Product Requirements', link: '/project/product-requirements' },
          { text: 'Implementation Roadmap', link: '/project/implementation-roadmap' },
          { text: 'Contributing Guide', link: '/project/contributing-guide' }
        ]
      }
    ],
    socialLinks: [
      { icon: 'github', link: 'https://github.com/basmulder03/git-project-sync' }
    ]
  }
})
