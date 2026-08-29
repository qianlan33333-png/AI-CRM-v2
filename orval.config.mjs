export default {
  health: {
    input: 'api/openapi.yaml',
    output: {
      target: 'web/src/api/generated/health.ts',
      client: 'fetch',
      mode: 'single',
      clean: true,
      prettier: true,
      override: {
        operations: {
          downloadChannelQRCode: {
            mutator: {
              path: 'web/src/api/orvalBlobFetch.ts',
              name: 'orvalBlobFetch',
            },
          },
        },
      },
    },
  },
};
