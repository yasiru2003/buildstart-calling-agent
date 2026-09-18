import React, { useEffect, useRef } from "react";

export const AudioMeter: React.FC<{
  label: string;
  stream: MediaStream | null | undefined;
}> = ({ label, stream }) => {
  const barRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    if (!stream) {
      if (barRef.current) barRef.current.style.width = "0%";
      return;
    }

    const AC =
      window.AudioContext ||
      (window as unknown as { webkitAudioContext?: typeof AudioContext })
        .webkitAudioContext;
    if (!AC) return;

    let ctx: AudioContext | null = null;
    let stopped = false;
    let animId: number;

    try {
      ctx = new AC();
      const src = ctx.createMediaStreamSource(stream);
      const analyser = ctx.createAnalyser();
      analyser.fftSize = 512;
      analyser.smoothingTimeConstant = 0.3;
      src.connect(analyser);
      const data = new Float32Array(analyser.fftSize);

      let lastPct = 0;
      const tick = () => {
        if (stopped) return;
        analyser.getFloatTimeDomainData(data);
        let sum = 0;
        for (let i = 0; i < data.length; i += 1) {
          sum += data[i] * data[i];
        }
        const rms = Math.sqrt(sum / data.length);
        const db =
          rms > 0.0001 ? Math.max(-60, Math.min(0, 20 * Math.log10(rms))) : -60;
        const targetPct = Math.max(
          0,
          Math.min(100, Math.round(((db + 60) / 60) * 100))
        );
        lastPct =
          targetPct > lastPct ? targetPct : Math.max(0, lastPct * 0.88);

        if (barRef.current) {
          barRef.current.style.width = `${Math.round(lastPct)}%`;
        }
        animId = requestAnimationFrame(tick);
      };
      animId = requestAnimationFrame(tick);

      return () => {
        stopped = true;
        cancelAnimationFrame(animId);
        try {
          src.disconnect();
          analyser.disconnect();
          ctx?.close();
        } catch {}
      };
    } catch {
      return;
    }
  }, [stream]);

  return (
    <div className="space-y-1">
      <p className="text-xs text-muted-foreground">{label}</p>
      <div className="h-2 overflow-hidden rounded-full bg-muted">
        <div
          ref={barRef}
          className="h-full bg-primary transition-[width] duration-75 ease-out"
          style={{ width: "0%" }}
        />
      </div>
    </div>
  );
};
