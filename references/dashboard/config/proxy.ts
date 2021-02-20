export default {
  dev: {
    '/api/': {
      target: 'http://localhost:38081/',
      changeOrigin: true,
    },
  },
  test: {
    '/api/': {
      target: 'http://localhost:38081/',
      changeOrigin: true,
    },
  },
  pre: {
    '/api/': {
      target: 'http://localhost:38081/',
      changeOrigin: true,
    },
  },
};
