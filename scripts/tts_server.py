#!/usr/bin/env python3
"""
High-Performance Local Sinhala Neural TTS Server
Powered by Piper TTS (si_LK-sinhala-medium ONNX)
Optimized with natural human cadence, conversational filler pauses, and phonetic normalization.
"""

import io
import os
import re
import sys
import wave
import numpy as np
from huggingface_hub import hf_hub_download
from piper import PiperVoice
from piper.config import SynthesisConfig
from flask import Flask, request, Response, jsonify

app = Flask(__name__)

print("🇱🇰 Loading Sinhala Neural Piper TTS ONNX model...")
try:
    token = os.environ.get("HF_TOKEN", "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")
    onnx_path = hf_hub_download('chan4lk/piper-tts-sinhala', 'si_LK-sinhala-medium.onnx', token=token)
    json_path = hf_hub_download('chan4lk/piper-tts-sinhala', 'si_LK-sinhala-medium.onnx.json', token=token)
    voice = PiperVoice.load(onnx_path, config_path=json_path)
    sample_rate = voice.config.sample_rate or 22050
    # Slightly relaxed cadence (1.06) + natural phoneme duration variance for clear articulation
    syn_config = SynthesisConfig(length_scale=1.06, noise_scale=0.7, noise_w_scale=0.85)
    print(f"✅ Sinhala Neural TTS Model Loaded & Ready! (Sample Rate: {sample_rate}Hz)")
except Exception as e:
    print(f"❌ Error loading Sinhala Piper voice: {e}")
    sys.exit(1)


NUM_MAP = {
    '0': 'බිංදුව', '1': 'එක', '2': 'දෙක', '3': 'තුන', '4': 'හතර',
    '5': 'පහ', '6': 'හය', '7': 'හත', '8': 'අට', '9': 'නවය'
}


def normalize_text(text: str) -> str:
    """Preprocess and clean text for crystal-clear Sinhala phonemization."""
    if not text:
        return ""

    # Remove markdown symbols and emojis
    text = re.sub(r'[*#_`~]', '', text)

    # Convert numeric digits to spoken Sinhala words
    for digit, word in NUM_MAP.items():
        text = text.replace(digit, f" {word} ")

    # Normalize multiple dots into comma pauses for natural breathing rhythm
    text = re.sub(r'\.{2,}', ', ', text)
    text = re.sub(r'\s+', ' ', text).strip()
    return text


@app.route("/health", methods=["GET"])
def health():
    return jsonify({
        "status": "ok",
        "engine": "piper-tts",
        "model": "chan4lk/piper-tts-sinhala",
        "sample_rate": sample_rate
    })


@app.route("/v1/audio/speech", methods=["POST"])
@app.route("/synthesize", methods=["POST"])
def synthesize():
    data = request.get_json(force=True, silent=True) or {}
    raw_text = data.get("input") or data.get("text") or ""
    text = normalize_text(raw_text)
    if not text:
        return Response(b"", mimetype="audio/wav")

    try:
        chunks = []
        for chunk in voice.synthesize(text, syn_config=syn_config):
            chunks.append(chunk.audio_int16_array)

        if not chunks:
            return Response(b"", mimetype="audio/wav")

        combined = np.concatenate(chunks)

        buf = io.BytesIO()
        with wave.open(buf, "wb") as wav_file:
            wav_file.setnchannels(1)
            wav_file.setsampwidth(2)
            wav_file.setframerate(sample_rate)
            wav_file.writeframes(combined.tobytes())

        buf.seek(0)
        return Response(buf.read(), mimetype="audio/wav")
    except Exception as e:
        print(f"Synthesis error: {e}")
        return jsonify({"error": str(e)}), 500


if __name__ == "__main__":
    port = int(os.environ.get("PORT", 5050))
    print(f"🚀 Sinhala Neural TTS server listening on http://127.0.0.1:{port}")
    app.run(host="127.0.0.1", port=port, debug=False)
