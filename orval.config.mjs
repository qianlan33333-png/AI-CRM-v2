export default {
  health: {
    input: "api/openapi.yaml",
    output: {
      target: "web/src/api/generated/health.ts",
      client: "fetch",
      mode: "tags-split",
      clean: true,
      prettier: true,
      override: {
        operations: {
          downloadChannelQRCode: {
            mutator: {
              path: "web/src/api/orvalBlobFetch.ts",
              name: "orvalBlobFetch",
            },
          },
        },
      },
    },
  },
};
