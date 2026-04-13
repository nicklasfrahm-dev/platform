import matplotlib.pyplot as plt
import numpy as np

def calculate_costs(
    gpu_cost_usd=780,
    other_hardware_usd=300,
    lifespan_years=3,
    elec_price_kwh=0.38,
    idle_power_w=150,
    load_power_w=500,
    input_tok_s=21.46,
    output_tok_s=197.51,
    total_tok_s=218.97
):
    total_hardware = gpu_cost_usd + other_hardware_usd
    depreciation_per_hr = total_hardware / (lifespan_years * 365 * 24)
    idle_cost_hr = (idle_power_w / 1000) * elec_price_kwh
    load_cost_hr = (load_power_w / 1000) * elec_price_kwh
    tokens_per_hr = total_tok_s * 3600
    delta_power_w = load_power_w - idle_power_w
    delta_cost_hr = (delta_power_w / 1000) * elec_price_kwh
    base_idle_hr = depreciation_per_hr + idle_cost_hr
    total_hourly_cost = base_idle_hr + delta_cost_hr
    avg_cost_per_1m = (total_hourly_cost / tokens_per_hr) * 1000000
    input_cost_1m = avg_cost_per_1m / 2.8
    output_cost_1m = input_cost_1m * 3.0
    cache_multiplier = 0.1
    cached_cost_in = input_cost_1m * cache_multiplier
    cached_cost_out = output_cost_1m * cache_multiplier
    return {
        "input_1m": input_cost_1m,
        "output_1m": output_cost_1m,
        "cached_in_1m": cached_cost_in,
        "cached_out_1m": cached_cost_out,
        "depreciation_hr": depreciation_per_hr,
        "idle_hr": idle_cost_hr,
        "load_hr": load_cost_hr,
        "total_hourly": total_hourly_cost,
        "tokens_per_hr": tokens_per_hr
    }

def generate_roi_plot(results, elec_price_kwh, cloud_cost_per_1m=1.0):
    utilization_range = np.linspace(0.01, 1.0, 100)
    roi_values = []
    for u in utilization_range:
        hourly_cost = results['depreciation_hr'] + \
                      (results['idle_hr'] * (1-u)) + \
                      (results['load_hr'] * u)
        tokens_per_hr = results['tokens_per_hr'] * u
        if tokens_per_hr <= 0:
            roi_values.append(0)
            continue
        cost_per_1m = (hourly_cost / tokens_per_hr) * 1000000
        savings_per_1m = cloud_cost_per_1m - cost_per_1m
        annual_savings = (savings_per_1m / 1000000) * tokens_per_hr * 8760
        roi_annual = (annual_savings / 1080) * 100
        roi_values.append(roi_annual)
    plt.figure(figsize=(10, 6))
    plt.plot(utilization_range * 100, roi_values, label='Annual ROI (%)', color='green', linewidth=2)
    plt.title('Homelab LLM ROI vs. Cloud (Gemma 4 @ 10% Idle Delta)', fontsize=14)
    plt.xlabel('System Utilization (%)', fontsize=12)
    plt.ylabel('Annual ROI (%)', fontsize=12)
    plt.grid(True, which='both', linestyle='--', alpha=0.5)
    plt.axhline(0, color='black', lw=1)
    plt.legend()
    plt.savefig('docs/img/roi_model.png')

def generate_comparison_plot(res, claude_sonnet, claude_haiku):
    categories = ['Input', 'Output', 'Input (Cached)', 'Output (Cached)']
    gemma_vals = [res['input_1m'], res['output_1m'], res['cached_in_1m'], res['cached_out_1m']]
    sonnet_vals = [claude_sonnet['input'], claude_sonnet['output'], claude_sonnet['cached_in'], claude_sonnet['cached_out']]
    haiku_vals = [claude_haiku['input'], claude_haiku['output'], claude_haiku['cached_in'], claude_haiku['cached_out']]

    x = np.arange(len(categories))
    width = 0.25

    fig, ax = plt.subplots(figsize=(12, 7))
    rects1 = ax.bar(x - width, gemma_vals, width, label='Gemma 4 (Homelab)', color='#4285F4')
    rects2 = ax.bar(x, haiku_vals, width, label='Claude 3.5 Haiku', color='#FBBC05')
    rects3 = ax.bar(x + width, sonnet_vals, width, label='Claude 3.5 Sonnet', color='#EA4335')

    ax.set_ylabel('Cost per 1M Tokens (USD)', fontsize=12)
    ax.set_title('Token Cost Comparison: Gemma 4 vs. Claude API', fontsize=14)
    ax.set_xticks(x)
    ax.set_xticklabels(categories)
    ax.legend()
    ax.grid(axis='y', linestyle='--', alpha=0.7)

    def autolabel(rects):
        for rect in rects:
            height = rect.get_height()
            ax.annotate(f'${height:.2f}',
                        xy=(rect.get_x() + rect.get_width() / 2, height),
                        xytext=(0, 3),
                        textcoords="offset points",
                        ha='center', va='bottom', fontsize=9)

    autolabel(rects1)
    autolabel(rects2)
    autolabel(rects3)

    fig.tight_layout()
    plt.savefig('docs/img/token_cost_comparison.png')

if __name__ == "__main__":
    res = calculate_costs(elec_price_kwh=0.38)
    
    claude_sonnet = {"input": 3.00, "output": 15.00, "cached_in": 0.30, "cached_out": 1.50}
    claude_haiku = {"input": 0.80, "output": 4.00, "cached_in": 0.16, "cached_out": 0.40}

    print(f"Gemma 4 Input: {res['input_1m']:.4f}")
    print(f"Gemma 4 Output: {res['output_1m']:.4f}")
    print(f"Gemma 4 Cached_In: {res['cached_in_1m']:.4f}")
    print(f"Gemma 4 Cached_Out: {res['cached_out_1m']:.4f}")
    print(f"\n--- Savings vs Claude 3.5 Haiku ---")
    print(f"Input Savings:  ${(claude_haiku['input'] - res['input_1m']):.4f}/1M")
    print(f"Output Savings: ${(claude_haiku['output'] - res['output_1m']):.4f}/1M")
    print(f"----------------------------------")
    
    generate_roi_plot(res, 0.38)
    generate_comparison_plot(res, claude_sonnet, claude_haiku)
    print("Plots saved to docs/img/")
