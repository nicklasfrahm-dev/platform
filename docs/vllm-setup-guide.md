# Setting up Crush with a private vLLM instance

To configure Crush to use your private vLLM instance, you need to edit your `crush.json` file located at `~/.local/share/crush/crush.json`.

## Configuration Steps

1. **Identify your vLLM endpoint**: This is the URL where your vLLM server is running (e.g., `http://your-server-ip:8000/v1`).
2. **Edit `crush.json`**: Add or update the `providers` section with a `vllm` entry using the `openai-compat` type.

### Example Configuration (Based on your setup)

```json
{
  "$schema": "https://charm.land/crush.json",
  "agent": {
    "enabled": true
  },
  "tools": {
    "enabled": true,
    "mode": "confirm"
  },
  "providers": {
    "vllm": {
      "type": "openai-compat",
      "base_url": "https://llm.cph02.nicklasfrahm.dev/v1",
      "api_key": "dummy",
      "models": [
        {
          "id": "cyankiwi/gemma-4-26B-A4B-it-AWQ-4bit",
          "name": "Gemma 4 26B A4B AWQ 4bit",

          // Maximum context window and default max tokens to allow for long conversations and tool calls.
          "context_window": 131072,
          "default_max_tokens": 16384,

          // Cost savings compared to Claude Haiku (per 1M tokens).
          "cost_per_1m_in": 0.6953,
          "cost_per_1m_out": 3.6859,
          "cost_per_1m_in_cached": 0.0695,
          "cost_per_1m_out_cached": 0.3686,

          // Ensure the thinking template is used for this model to enable auto-tool-choice.
          "extra_body": {
            "chat_template_kwargs": {
              "enable_thinking": true
            }
          }
        }
      ]
    }
  },
  "models": {
    "large": {
      "model": "cyankiwi/gemma-4-26B-A4B-it-AWQ-4bit",
      "provider": "vllm"
    },
    "small": {
      "model": "cyankiwi/gemma-4-26B-A4B-it-AWQ-4bit",
      "provider": "vllm"
    }
  },
  "recent_models": {
    "large": [
      {
        "model": "cyankiwi/gemma-4-26B-A4B-it-AWQ-4bit",
        "provider": "vllm"
      }
    ],
    "small": [
      {
        "model": "cyankiwi/gemma-4-26B-A4B-it-AWQ-4bit",
        "provider": "vllm"
      }
    ]
  }
}
```

### Example opencode Configuration

If you are using opencode, your configuration in `~/.config/opencode/opencode.json` might look like this:

```json
{
  "$schema": "https://opencode.ai/config.json",
  "provider": {
    "vllm": {
      "npm": "@ai-sdk/openai-compatible",
      "name": "vLLM",
      "options": {
        "baseURL": "https://llm.cph02.nicklasfrahm.dev/v1"
      },
      "models": {
        "cyankiwi/gemma-4-26B-A4B-it-AWQ-4bit": {
          "name": "Gemma 4 MoE",
          "limit": {
            "context": 131072,
            "output": 16384
          }
        }
      }
    }
  }
}
```


- `type`: Set this to `"openai-compat"` as vLLM implements the OpenAI API specification.
- `base_url`: The full URL to your vLLM `/v1` endpoint.
- `api_key`: If your instance requires authentication, provide the key here. Otherwise, use a dummy string like `"dummy"`.
- `models`: A list of models available on your vLLM instance. The `id` must match the model identifier used by your vLLM server.
