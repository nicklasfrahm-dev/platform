# Setting up Crush with a private vLLM instance

To configure Crush to use your private vLLM instance, you need to edit your `crush.json` file located at `~/.local/share/crush/crush.json`.

## Configuration Steps

1. **Identify your vLLM endpoint**: This is the URL where your vLLM server is running (e.g., `http://your-server-ip:8000/v1`).
2. **Edit `crush.json`**: Add or update the `providers` section with a `vllm` entry using the `openai-compat` type.

### Example Configuration

```json
{
  "$schema": "https://charm.land/crush.json",
  "providers": {
    "vllm": {
      "type": "openai-compat",
      "base_url": "http://YOUR_PRIVATE_IP:8000/v1",
      "api_key": "your-api-key-if-any",
      "models": [
        {
          "id": "model-name-in-vllm",
          "name": "Friendly Model Name",
          "context_window": 32768,
          "default_max_tokens": 4096
        }
      ]
    }
  }
}
```

## Key Parameters

- `type`: Set this to `"openai-compat"` as vLLM implements the OpenAI API specification.
- `base_url`: The full URL to your vLLM `/v1` endpoint.
- `api_key`: If your instance requires authentication, provide the key here. Otherwise, use a dummy string like `"dummy"`.
- `models`: A list of models available on your vLLM instance. The `id` must match the model identifier used by your vLLM server.
