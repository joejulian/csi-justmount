module.exports = {
  extends: ['@commitlint/config-conventional'],
  rules: {
    'body-max-line-length': [0],
    'subject-case': [2, 'never', ['sentence-case', 'start-case', 'pascal-case']],
  },
};
