import sounddevice as sd
import wave
import sys
import threading
import numpy as np

SAMPLE_RATE = 16000  # 16 kHz
CHANNELS = 1         # Mono
FORMAT = 'int16'     # 16-bit PCM
OUTPUT_FILE = "output.wav"

stop_flag = threading.Event()
frames = []

def record_audio():
    with sd.InputStream(samplerate=SAMPLE_RATE, channels=CHANNELS, dtype=FORMAT) as stream:
        print("🎙️ Recording... (Press Enter to stop)")
        while not stop_flag.is_set():
            data, _ = stream.read(1024)
            frames.append(np.copy(data))  # ensure data is copied

def main():
    thread = threading.Thread(target=record_audio)
    thread.start()

    input()  # wait for Enter
    stop_flag.set()
    thread.join()

    # Flatten all frames to a single 1D array and convert to bytes
    audio_data = np.concatenate(frames)
    audio_bytes = audio_data.tobytes()

    with wave.open(OUTPUT_FILE, 'wb') as wf:
        wf.setnchannels(CHANNELS)
        wf.setsampwidth(2)  # 16-bit PCM
        wf.setframerate(SAMPLE_RATE)
        wf.writeframes(audio_bytes)

    print(f"✅ Saved recording to {OUTPUT_FILE}")

if __name__ == "__main__":
    try:
        main()
    except KeyboardInterrupt:
        sys.exit(0)
