# LLM Inference Benchmarks — KServe / vLLM on cph02

Benchmarks run against the OpenAI-compatible vLLM endpoint at `https://llm.cph02.nicklasfrahm.dev`.

Each benchmark sends **50 chat completion requests** at **2 req/s**, with each prompt requesting up to 200 completion tokens.

---

## System Specifications

| Component | Details |
|-----------|---------|
| Node | `deer` (Talos v1.12.6, kernel 6.18.18-talos) |
| GPU | NVIDIA GeForce RTX 3090 (Ampere, compute 8.6) |
| VRAM | 24,576 MiB |
| CPU | 4 vCPUs |
| RAM | ~15.5 GiB |
| CUDA runtime | 13.0 |
| NVIDIA driver | 580.126.20 |
| Container runtime | containerd 2.1.6 |
| vLLM image | `vllm/vllm-openai:v0.19.0` |

---

## Qwen/Qwen2.5-Coder-7B-Instruct-AWQ

**Model**: `Qwen/Qwen2.5-Coder-7B-Instruct-AWQ`
**Quantization**: AWQ 4-bit
**Config**: `--gpu-memory-utilization=0.9`, `--max-model-len=6144`, `--max-num-seqs=2`, `--enable-auto-tool-choice`

| Metric | Value |
|--------|-------|
| Requests | 50 |
| Successful | 21 |
| Failed (timeout) | 29 |
| Total time | 145.22 s |
| Request throughput | 0.14 req/s |
| Output token throughput | 27.90 tok/s |
| Total token throughput | 33.38 tok/s |
| Latency mean | 60.633 s |
| Latency median | 60.371 s |
| Latency stdev | 33.542 s |
| Latency P90 | 104.112 s |
| Latency P99 | 115.295 s |
| Latency min | 11.857 s |
| Latency max | 115.295 s |

> **Note**: 29 requests failed due to the 120 s client timeout. With `--max-num-seqs=2` and a 2 req/s injection rate the server queue depth grows faster than it drains, causing long tail latencies. This constraint is intentional to cap memory pressure on the single 24 GB GPU.

---

## cyankiwi/gemma-4-26B-A4B-it-AWQ-4bit

**Model**: `cyankiwi/gemma-4-26B-A4B-it-AWQ-4bit` (Google Gemma 4 27B MoE, 4B active parameters)
**Quantization**: AWQ 4-bit

**Status**: ❌ Incompatible with vLLM 0.19.0

**Root cause**: The `cyankiwi/gemma-4-26B-A4B-it-AWQ-4bit` model was quantized with HuggingFace `transformers` v5.x. vLLM 0.19.0 requires `transformers<5,>=4.56.0`, and the `gemma4` model architecture is not registered in the transformers 4.x series bundled in the vLLM container.

**Error**:
```
pydantic_core._pydantic_core.ValidationError: 1 validation error for ModelConfig
  Value error, The checkpoint you are trying to load has model type `gemma4`
  but Transformers does not recognize this architecture.
```

**Fix**: Either upgrade to a vLLM release that ships with `transformers>=5.0` support, or use a Gemma 4 AWQ checkpoint that was produced with the transformers 4.x API (e.g. a model with a `config.json` targeting `transformers==4.5x.y`).
