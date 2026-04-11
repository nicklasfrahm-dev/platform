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

## Qwen/Qwen2.5-32B-Instruct-AWQ

**Model**: `Qwen/Qwen2.5-32B-Instruct-AWQ`
**Quantization**: AWQ 4-bit
**Config**: `--gpu-memory-utilization=0.95`, `--max-model-len=4096`, `--max-num-seqs=2`

| Metric | Value |
|--------|-------|
| Requests | 50 |
| Successful | 4 |
| Failed (timeout) | 46 |
| Total time | 144.70 s |
| Request throughput | 0.03 req/s |
| Output token throughput | 5.53 tok/s |
| Total token throughput | 6.61 tok/s |
| Latency mean | 87.200 s |
| Latency median | 87.164 s |
| Latency stdev | 33.309 s |
| Latency P90 | 116.178 s |
| Latency P99 | 116.178 s |
| Latency min | 58.296 s |
| Latency max | 116.178 s |

> **Note**: The 32B model is ~4.6× larger than the 7B model, resulting in ~5× lower output throughput (5.53 vs 27.90 tok/s) and ~46% higher mean latency (87 vs 61 s). The 46 failures are due to the same queue-depth issue as above. Both models run with `--max-num-seqs=2` which severely limits concurrency; a lower request rate (≤0.2 req/s) would eliminate queue build-up for the 32B model.

---

## cyankiwi/gemma-4-26B-A4B-it-AWQ-4bit

**Model**: `cyankiwi/gemma-4-26B-A4B-it-AWQ-4bit` (Google Gemma 4 27B MoE, 4B active parameters)
**Quantization**: AWQ 4-bit

**Status**: ❌ Incompatible with vLLM 0.19.0

**Root cause**: The `cyankiwi/gemma-4-26B-A4B-it-AWQ-4bit` model requires HuggingFace `transformers` v5.x for the `gemma4` architecture. vLLM 0.19.0 requires `transformers<5,>=4.56.0` and the `gemma4` model type is not registered in the transformers 4.x series bundled in the vLLM container.

**Error**:
```
pydantic_core._pydantic_core.ValidationError: 1 validation error for ModelConfig
  Value error, The checkpoint you are trying to load has model type `gemma4`
  but Transformers does not recognize this architecture.
```

**Fix**: Upgrade to a vLLM release that ships with `transformers>=5.0` support.
