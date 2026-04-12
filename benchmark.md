# LLM Inference Benchmarks — KServe / vLLM on cph02

Benchmarks run against the OpenAI-compatible vLLM endpoint at `https://llm.cph02.nicklasfrahm.dev`.

Each benchmark sends **50 chat completion requests** at **2 req/s**, with each prompt requesting up to 200 completion tokens.

**Backpressure**: All runs use a client-side `--max-concurrent` check against the `/load` endpoint. Requests are skipped immediately (0 ms) when `server_load >= max_concurrent`, preventing unbounded queue growth on the server. The server also runs with `--enable-server-load-tracking` and a `BackpressureMiddleware` that returns HTTP 503 before enqueueing when at capacity.

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
| vLLM image (Qwen) | `vllm/vllm-openai:v0.19.0` |
| vLLM image (Gemma 4) | `vllm/vllm-openai:gemma4` (0.19.1.dev6, transformers ≥ 5.5.0) |

---

## Qwen/Qwen2.5-Coder-7B-Instruct-AWQ

**Model**: `Qwen/Qwen2.5-Coder-7B-Instruct-AWQ`
**Quantization**: AWQ 4-bit
**Config**: `--gpu-memory-utilization=0.9`, `--max-model-len=6144`, `--max-num-seqs=2`, `--enable-auto-tool-choice`
**Backpressure**: not applied (baseline run)

| Metric | Value |
|--------|-------|
| Requests | 50 |
| Successful | 21 |
| Skipped (backpressure) | — |
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

> Without backpressure, 29 requests timed out (120 s client timeout) as the server queue grew faster than it drained at 2 req/s with `--max-num-seqs=2`.

---

## Qwen/Qwen2.5-32B-Instruct-AWQ

**Model**: `Qwen/Qwen2.5-32B-Instruct-AWQ`
**Quantization**: AWQ 4-bit
**Config**: `--gpu-memory-utilization=0.95`, `--max-model-len=4096`, `--max-num-seqs=2`
**Backpressure**: not applied (baseline run)

| Metric | Value |
|--------|-------|
| Requests | 50 |
| Successful | 4 |
| Skipped (backpressure) | — |
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

> The 32B dense model is ~5× slower than the 7B model. 46/50 requests timed out.

---

## cyankiwi/gemma-4-26B-A4B-it-AWQ-4bit

**Model**: `cyankiwi/gemma-4-26B-A4B-it-AWQ-4bit`  
**Architecture**: Google Gemma 4 27B MoE — 26B total / **4B active** parameters per forward pass  
**Quantization**: compressed-tensors (AWQ 4-bit)  
**Config**: `--gpu-memory-utilization=0.95`, `--max-model-len=4096`, `--max-num-seqs=2`  
**vLLM image**: `vllm/vllm-openai:gemma4` (requires transformers ≥ 5.5.0 for `model_type: gemma4`)  
**Backpressure**: `--enable-server-load-tracking` + `BackpressureMiddleware`, `--max-concurrent=4` on client

| Metric | Value |
|--------|-------|
| Requests | 50 |
| Successful | 26 |
| Skipped (backpressure) | 24 |
| Failed (error) | 0 |
| Total time | 26.33 s |
| Request throughput | 0.99 req/s |
| Output token throughput | **197.51 tok/s** |
| Total token throughput | 218.97 tok/s |
| Latency mean | 1.833 s |
| Latency median | 1.836 s |
| Latency stdev | 0.015 s |
| Latency P90 | 1.845 s |
| Latency P99 | 1.853 s |
| Latency min | 1.772 s |
| Latency max | 1.853 s |

> The MoE architecture is the key differentiator: with only **4B active parameters** per forward pass (vs. 7B for Qwen-Coder), Gemma 4 is ~7× faster in output token throughput (197 vs. 28 tok/s) despite having 26B total parameters. The backpressure mechanism ensured zero failures — 24 requests were rejected immediately rather than queuing, keeping P99 latency at 1.85 s vs. 115 s in the baseline runs.
