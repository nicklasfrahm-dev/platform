# LLM Inference Benchmarks — KServe / vLLM on cph02

Benchmarks run against the OpenAI-compatible vLLM endpoint at `https://llm.cph02.nicklasfrahm.dev`.

Each benchmark sends **50 chat completion requests** at **2 req/s**, with each prompt requesting up to 200 completion tokens.

**Backpressure**: All runs use `--enable-server-load-tracking` + a `BackpressureMiddleware` on the server (returns HTTP 503 immediately when `server_load_metrics >= max_num_seqs`) and a client-side `/load` check (`--max-concurrent=4`). Requests are skipped at zero cost rather than queuing indefinitely.

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
| vLLM image | `vllm/vllm-openai:gemma4` (0.19.1.dev6, transformers ≥ 5.5.0) |

---

## Qwen/Qwen2.5-Coder-7B-Instruct-AWQ — awq

**Model**: `Qwen/Qwen2.5-Coder-7B-Instruct-AWQ`
**Quantization**: `awq`
**Config**: `--gpu-memory-utilization=0.9`, `--max-model-len=6144`, `--max-num-seqs=2`, `--enable-auto-tool-choice`

| Metric | Value |
|--------|-------|
| Requests | 50 |
| Successful | 4 |
| Skipped (backpressure) | 46 |
| Failed (error) | 0 |
| Total time | 25.57 s |
| Request throughput | 0.16 req/s |
| Output token throughput | 31.28 tok/s |
| Total token throughput | 37.27 tok/s |
| Latency mean | 12.185 s |
| Latency median | 12.182 s |
| Latency stdev | 0.237 s |
| Latency P90 | 12.438 s |
| Latency P99 | 12.438 s |
| Latency min | 11.937 s |
| Latency max | 12.438 s |

---

## Qwen/Qwen2.5-Coder-7B-Instruct-AWQ — awq_marlin

**Model**: `Qwen/Qwen2.5-Coder-7B-Instruct-AWQ`
**Quantization**: `awq_marlin` (NVIDIA Marlin GEMM kernels)
**Config**: `--gpu-memory-utilization=0.9`, `--max-model-len=6144`, `--max-num-seqs=2`, `--enable-auto-tool-choice`

| Metric | Value |
|--------|-------|
| Requests | 50 |
| Successful | 33 |
| Skipped (backpressure) | 17 |
| Failed (error) | 0 |
| Total time | 25.94 s |
| Request throughput | 1.27 req/s |
| Output token throughput | **249.38 tok/s** |
| Total token throughput | 297.87 tok/s |
| Latency mean | 1.383 s |
| Latency median | 1.382 s |
| Latency stdev | 0.186 s |
| Latency P90 | 1.396 s |
| Latency P99 | 1.993 s |
| Latency min | 0.592 s |
| Latency max | 1.993 s |

---

## cyankiwi/gemma-4-26B-A4B-it-AWQ-4bit

**Model**: `cyankiwi/gemma-4-26B-A4B-it-AWQ-4bit`
**Architecture**: Google Gemma 4 27B MoE — 26B total / **4B active** parameters per forward pass
**Quantization**: `compressed-tensors` (AWQ 4-bit)
**Config**: `--gpu-memory-utilization=0.95`, `--max-model-len=4096`, `--max-num-seqs=2`
**vLLM image**: `vllm/vllm-openai:gemma4` (requires transformers ≥ 5.5.0)

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
