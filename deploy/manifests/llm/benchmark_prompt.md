# Create an `llm` chart

1. Most configuration should live with sane defaults in the values.yaml.
2. The chart should contain multiple named clusterservingruntimes, e.g. `gemma4-runtime`.
3. The chart should create a single inferenceservice referencing one of the clusterservingruntimes, e.g. `gemma4-moe`.
4. The name of the runtimes should be prefixed with the release name
5. The name of the inferenceservice should be the same as the release name
6. There should be an http route that can be controlled via values.yaml to point to the inferenceservice, e.g. `gemma4-moe`

For the initial implementation, use the deploy/manifests/llm/gemma4 folder as a reference, but make sure that we implement best-practice labels and annotations, and that we remove any hardcoded values that should be configurable via values.yaml
