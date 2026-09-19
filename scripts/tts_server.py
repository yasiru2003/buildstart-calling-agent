#!/usr/bin/env python3
"""
Studio-Quality Sinhala & Multilingual Neural TTS Server
Powered by Microsoft Azure / Edge Neural Voices (si-LK-ThiliniNeural & si-LK-SameeraNeural)
with local Piper TTS (si_LK-sinhala-medium ONNX) as offline fallback.
"""

import asyncio
import base64
import io
import json
import os
import re
import sys
import threading
import wave
import websockets
import numpy as np
import requests
from flask import Flask, request, Response, jsonify

app = Flask(__name__)
piper_lock = threading.Lock()

# Voice hierarchy:
# 1. gemini-aoede / gemini-puck / gemini-charon (Google AI Studio Gemini Flash Native Voice - super-natural human voice)
# 2. si-LK-ThiliniNeural (Microsoft Female Neural - warm, natural conversational voice)
# 3. piper-ashoka (Local Human Voice by Ashoka Weerawardhana - 100% offline fallback)
GEMINI_API_KEY = os.environ.get("GEMINI_API_KEY", "")
DEFAULT_VOICE = os.environ.get("DEFAULT_VOICE", "gemini-aoede")
RATE_MODIFIER = "+4%"  # Conversational pace
PITCH_MODIFIER = "+1Hz"
ELEVENLABS_API_KEY = os.environ.get("ELEVENLABS_API_KEY", "")
ELEVENLABS_VOICE_ID = os.environ.get("ELEVENLABS_VOICE_ID", "21m00Tcm4TlvDq8ikWAM")

# Pre-load local Piper TTS models (Ashoka human voice)
piper_voices = {}
try:
    from huggingface_hub import hf_hub_download
    from piper import PiperVoice
    from piper.config import SynthesisConfig

    print("🇱🇰 Initializing Piper local neural models...")
    # 1. Intellisr Ashoka Weerawardhana model
    p_int_onnx = hf_hub_download('intellisr/sinhala-tts-piper-v1', 'model.onnx')
    p_int_json = hf_hub_download('intellisr/sinhala-tts-piper-v1', 'model.onnx.json')
    piper_voices['piper-intellisr'] = PiperVoice.load(p_int_onnx, config_path=p_int_json)
    piper_voices['piper-ashoka'] = piper_voices['piper-intellisr']

    # 2. UNICEF Ashoka medium model
    p_uni_onnx = hf_hub_download('unicef/piper-si_LK-ashoka-medium', 'si_LK-ashoka-medium.onnx')
    p_uni_json = hf_hub_download('unicef/piper-si_LK-ashoka-medium', 'si_LK-ashoka-medium.onnx.json')
    piper_voices['piper-unicef'] = PiperVoice.load(p_uni_onnx, config_path=p_uni_json)

    print("✅ Local Piper Human Voice models ready: ['piper-intellisr', 'piper-unicef']")
except Exception as e:
    print(f"⚠️ Piper models initialization notice: {e}")

try:
    import edge_tts
    print(f"✨ Microsoft Studio Neural TTS Engine Activated! (Default Voice: {DEFAULT_VOICE})")
except ImportError:
    print("❌ edge_tts library missing. Install via: pip install edge-tts")
    sys.exit(1)


NUM_MAP = {
    '0': 'බිංදුව', '1': 'එක', '2': 'දෙක', '3': 'තුන', '4': 'හතර',
    '5': 'පහ', '6': 'හය', '7': 'හත', '8': 'අට', '9': 'නවය'
}

# Automatic conversion of bookish/formal words into warm spoken Sinhala
COLLOQUIAL_MAP = [
    ('ඔබගේ', 'ඔයාගෙ'),
    ('ඔබට', 'ඔයාට'),
    ('ඔබව', 'ඔයාව'),
    ('ඔබ', 'ඔයා'),
    ('උදවු', 'උදව්'),
    ('පවසන්න', 'කියන්න'),
    ('හැකියි', 'පුළුවන්'),
    ('හැකිද', 'පුළුවන්ද'),
]


def normalize_text(text: str) -> str:
    """Preprocess and clean text for natural spoken Sinhala phonemization."""
    if not text:
        return ""

    # Strip markdown symbols, asterisks, brackets, and emojis
    text = re.sub(r'[*#_`~]', '', text)

    # Convert numeric digits to spoken Sinhala words
    for digit, word in NUM_MAP.items():
        text = text.replace(digit, f" {word} ")

    # Normalize bookish/written words to natural spoken Sinhala
    for formal, spoken in COLLOQUIAL_MAP:
        text = text.replace(formal, spoken)

    # Normalize multiple dots and dashes into natural breath pauses
    text = re.sub(r'\.{2,}', ', ', text)
    text = re.sub(r'[-–—]', ' ', text)
    text = re.sub(r'\s+', ' ', text).strip()
    return text


def synthesize_elevenlabs(text: str, voice_id: str = ELEVENLABS_VOICE_ID) -> bytes:
    """Synthesize speech using ElevenLabs Multilingual v2 if key is configured."""
    import requests
    url = f"https://api.elevenlabs.io/v1/text-to-speech/{voice_id}?output_format=mp3_44100_128"
    headers = {
        "xi-api-key": ELEVENLABS_API_KEY,
        "Content-Type": "application/json"
    }
    payload = {
        "text": text,
        "model_id": "eleven_multilingual_v2",
        "voice_settings": {
            "stability": 0.5,
            "similarity_boost": 0.75
        }
    }
    resp = requests.post(url, headers=headers, json=payload, timeout=8)
    if resp.status_code == 200:
        return resp.content
    raise RuntimeError(f"ElevenLabs error {resp.status_code}: {resp.text}")


def synthesize_gemini_tts(text: str, voice: str = "Aoede") -> bytes:
    """Generate human-lifelike speech directly using Google AI Studio Gemini Live Multimodal WebSocket."""
    if not GEMINI_API_KEY:
        raise ValueError("GEMINI_API_KEY is not configured")

    v_map = {
        "gemini-aoede": "Aoede", "gemini-puck": "Puck", "gemini-charon": "Charon",
        "gemini-kore": "Kore", "gemini-fenrir": "Fenrir",
        "aoede": "Aoede", "puck": "Puck", "charon": "Charon",
        "kore": "Kore", "fenrir": "Fenrir",
    }
    voice_name = v_map.get(voice.lower(), "Aoede")

    # 1. Primary: Gemini Multimodal Live WebSocket (Zero Daily Quota / True AI Studio Voice)
    async def _ws_synth():
        import websockets
        host = "generativelanguage.googleapis.com"
        ws_url = f"wss://{host}/ws/google.ai.generativelanguage.v1alpha.GenerativeService.BidiGenerateContent?key={GEMINI_API_KEY}"
        async with websockets.connect(ws_url, close_timeout=2) as ws:
            setup = {
                "setup": {
                    "model": "models/gemini-2.5-flash-native-audio-latest",
                    "generationConfig": {
                        "responseModalities": ["AUDIO"],
                        "speechConfig": {
                            "voiceConfig": {
                                "prebuiltVoiceConfig": {
                                    "voiceName": voice_name
                                }
                            }
                        }
                    }
                }
            }
            await ws.send(json.dumps(setup))
            await asyncio.wait_for(ws.recv(), timeout=4)  # setupComplete

            client_turn = {
                "clientContent": {
                    "turns": [{
                        "role": "user",
                        "parts": [{"text": f"Please read the following text aloud warmly in Sinhala: {text}"}]
                    }],
                    "turnComplete": True
                }
            }
            await ws.send(json.dumps(client_turn))

            raw_pcm = bytearray()
            while True:
                msg = await asyncio.wait_for(ws.recv(), timeout=8)
                data = json.loads(msg)
                server_turn = data.get("serverContent", {}).get("modelTurn", {})
                for p in server_turn.get("parts", []):
                    if "inlineData" in p:
                        raw_pcm.extend(base64.b64decode(p["inlineData"]["data"]))
                if data.get("serverContent", {}).get("turnComplete"):
                    break

            if len(raw_pcm) > 0:
                buf = io.BytesIO()
                with wave.open(buf, "wb") as wf:
                    wf.setnchannels(1)
                    wf.setsampwidth(2)
                    wf.setframerate(24000)
                    wf.writeframes(raw_pcm)
                return buf.getvalue()
            return b""

    try:
        loop = asyncio.new_event_loop()
        asyncio.set_event_loop(loop)
        try:
            res = loop.run_until_complete(_ws_synth())
            if res and len(res) > 100:
                return res
        finally:
            loop.close()
    except Exception as ws_err:
        print(f"⚠️ Gemini Live WS error: {ws_err}, trying REST...")

    # 2. Secondary: REST Endpoint (gemini-3.1-flash-tts-preview)
    url = f"https://generativelanguage.googleapis.com/v1beta/models/gemini-3.1-flash-tts-preview:generateContent?key={GEMINI_API_KEY}"
    payload = {
        "contents": [{"parts": [{"text": text}]}],
        "generationConfig": {
            "responseModalities": ["AUDIO"],
            "speechConfig": {
                "voiceConfig": {
                    "prebuiltVoiceConfig": {
                        "voiceName": voice_name
                    }
                }
            }
        }
    }
    resp = requests.post(url, json=payload, timeout=8)
    if resp.status_code == 200:
        res_json = resp.json()
        candidates = res_json.get("candidates", [])
        if candidates and "content" in candidates[0] and "parts" in candidates[0]["content"]:
            for part in candidates[0]["content"]["parts"]:
                if "inlineData" in part and "data" in part["inlineData"]:
                    raw_pcm = base64.b64decode(part["inlineData"]["data"])
                    buf = io.BytesIO()
                    with wave.open(buf, "wb") as wf:
                        wf.setnchannels(1)
                        wf.setsampwidth(2)
                        wf.setframerate(24000)
                        wf.writeframes(raw_pcm)
                    return buf.getvalue()
    raise RuntimeError(f"Gemini Live error: {resp.text[:120]}")


def synthesize_edge_tts(text: str, voice: str = DEFAULT_VOICE, rate: str = RATE_MODIFIER, pitch: str = PITCH_MODIFIER) -> bytes:
    """Synthesize speech using Microsoft Edge Neural voices (si-LK-ThiliniNeural / si-LK-SameeraNeural)."""
    async def _synth():
        comm = edge_tts.Communicate(text, voice, rate=rate, pitch=pitch)
        buf = io.BytesIO()
        async for chunk in comm.stream():
            if chunk.get("type") == "audio" and "data" in chunk:
                buf.write(chunk["data"])
        return buf.getvalue()

    loop = asyncio.new_event_loop()
    try:
        asyncio.set_event_loop(loop)
        return loop.run_until_complete(_synth())
    finally:
        loop.close()


def synthesize_piper(text: str, voice_key: str = "piper-intellisr") -> bytes:
    """Synthesize speech using local Piper ONNX models (Ashoka Weerawardhana)."""
    p_voice = piper_voices.get(voice_key) or piper_voices.get("piper-intellisr") or piper_voices.get("piper-unicef")
    if not p_voice:
        raise RuntimeError(f"Piper voice '{voice_key}' not found and no piper voices loaded")

    with piper_lock:
        chunks = list(p_voice.synthesize(text))
    if not chunks:
        return b""

    # Use audio_int16_bytes or audio_int16_array
    if hasattr(chunks[0], "audio_int16_bytes"):
        all_raw = b"".join(c.audio_int16_bytes for c in chunks)
    else:
        all_raw = np.concatenate([c.audio_int16_array for c in chunks]).tobytes()

    s_rate = p_voice.config.sample_rate or 16000
    buf = io.BytesIO()
    with wave.open(buf, "wb") as wav_file:
        wav_file.setnchannels(1)
        wav_file.setsampwidth(2)
        wav_file.setframerate(s_rate)
        wav_file.writeframes(all_raw)

    buf.seek(0)
    return buf.read()


@app.route("/health", methods=["GET"])
def health():
    return jsonify({
        "status": "ok",
        "primary_engine": "google-aistudio-gemini-tts" if GEMINI_API_KEY else "microsoft-neural",
        "default_voice": DEFAULT_VOICE,
        "available_voices": [
            "gemini-aoede (Google AI Studio Super-Natural Female)",
            "gemini-puck (Google AI Studio Super-Natural Male)",
            "gemini-charon (Google AI Studio Super-Natural Deep Male)",
            "si-LK-ThiliniNeural (Microsoft Female Neural - Free)",
            "si-LK-SameeraNeural (Male Neural - Free)",
            "piper-ashoka (Ashoka Weerawardhana Human Voice - 100% Offline & Free)"
        ]
    })


@app.route("/v1/audio/speech", methods=["POST"])
@app.route("/synthesize", methods=["POST"])
def synthesize():
    data = request.get_json(force=True, silent=True) or {}
    raw_text = data.get("input") or data.get("text") or ""
    voice = data.get("voice") or DEFAULT_VOICE

    text = normalize_text(raw_text)
    if not text:
        return Response(b"", mimetype="audio/mpeg")

    # 0. Primary: Google AI Studio Gemini Flash Native Voice (Super-Natural Human Voice)
    if GEMINI_API_KEY and ("gemini" in voice.lower() or voice.lower() in ["aoede", "puck", "charon", "kore", "fenrir"]):
        try:
            audio_wav = synthesize_gemini_tts(text, voice=voice)
            if audio_wav and len(audio_wav) > 100:
                return Response(audio_wav, mimetype="audio/wav")
        except Exception as g_err:
            print(f"⚠️ Gemini Flash Voice error: {g_err}, falling back to Edge Neural...")

    # If piper voice is requested directly
    if "piper" in voice.lower() or "ashoka" in voice.lower():
        try:
            audio_wav = synthesize_piper(text, voice_key=voice)
            if audio_wav:
                return Response(audio_wav, mimetype="audio/wav")
        except Exception as p_err:
            print(f"⚠️ Piper synthesis error: {p_err}, falling back to Edge Neural...")

    # Secondary: High-fidelity Microsoft Neural Voice (Thilini / Sameera)
    edge_voice = voice if "si-lk" in voice.lower() else "si-LK-ThiliniNeural"
    try:
        audio_mp3 = synthesize_edge_tts(text, voice=edge_voice)
        if audio_mp3 and len(audio_mp3) > 100:
            return Response(audio_mp3, mimetype="audio/mpeg")
    except Exception as edge_err:
        print(f"⚠️ Edge Neural TTS error: {edge_err}, falling back to Piper...")

    # Fallback: Local Piper ONNX
    try:
        audio_wav = synthesize_piper(text)
        if audio_wav:
            return Response(audio_wav, mimetype="audio/wav")
    except Exception as piper_err:
        print(f"❌ Piper fallback error: {piper_err}")

    return jsonify({"error": "Synthesis failed on all engines"}), 500


if __name__ == "__main__":
    port = int(os.environ.get("PORT", 5050))
    print(f"🚀 Studio Neural TTS server listening on http://127.0.0.1:{port} (Voice: {DEFAULT_VOICE})")
    app.run(host="127.0.0.1", port=port, debug=False)
