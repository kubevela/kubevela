const Configuration = {
  /*
   * Resolve and load @commitlint/config-conventional from node_modules.
   * Referenced packages must be installed
   */
  extends: ['@commitlint/config-conventional'],
  /*
   * Any rules defined here will override rules from @commitlint/config-conventional
   */
  rules: {
    'type-enum': [
			2,
			'always',
			[
				'Build',
				'Chore',
				'CI',
				'Docs',
				'Feat',
				'Fix',
				'Perf',
				'Refactor',
				'Revert',
				'Style',
				'Test',
			],
		],
    'type-case': [2, 'never', 'lower-case'],
  },
};

module.exports = Configuration;