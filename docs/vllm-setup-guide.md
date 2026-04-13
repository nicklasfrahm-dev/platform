# Setting up Crush with a private vLLM instance

To configure Crush to use your private vLLM instance, you need to edit your `crush.json` file located at `~/.local/share/crush/crush.json`.

## Configuration Steps

1. **Identify your vLLM endpoint**: This is the URL where your vLLM server is running (e.g., `http://your-server-ip:8000/v1`).
2. **Edit `crush.json`**: Add or update the `providers` section with a `vllm` entry using the `openai-compat` type.

### Example Configuration (Based on your setup)

```json
{
  "$schema": "https://charm.land/crush.json",
  "providers": {
    "vllm": {
      "type": "openai-compat",
      "base_url": "https://llm.cph02.nicklasfrahm.dev/v1",
      "api_key": "dummy",
      "models": [
        {
          "id": "cyankiwi/gemma-4-26B-A4B-it-AWQ-4bit",
          "name": "Gemma 4 26B A4B AWQ 4bit",
          "context_window": 131072,
          "default_max_tokens": 8192,
          "extra_body": {
            "chat_template_kwargs": {
              "enable_thinking": true
            }
          }
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
