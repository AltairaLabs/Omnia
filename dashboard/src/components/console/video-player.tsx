"use client";

import { useState, useRef, useEffect, useCallback } from "react";
import { Play, Pause, Maximize, Minimize, Volume2, VolumeX } from "lucide-react";
import { cn } from "@/lib/utils";
import { Slider } from "@/components/ui/slider";
import { Button } from "@/components/ui/button";

interface VideoPlayerProps {
  src: string;
  type?: string;
  filename?: string;
  fileSize?: number;
  className?: string;
}

function formatTime(seconds: number): string {
  if (!isFinite(seconds) || isNaN(seconds)) return "0:00";
  const mins = Math.floor(seconds / 60);
  const secs = Math.floor(seconds % 60);
  return `${mins}:${secs.toString().padStart(2, "0")}`;
}

function formatFileSize(bytes: number): string {
  if (bytes < 1024) return `${bytes} B`;
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`;
  return `${(bytes / (1024 * 1024)).toFixed(1)} MB`;
}

export function VideoPlayer({ src, type, filename, fileSize, className }: Readonly<VideoPlayerProps>) {
  const videoRef = useRef<HTMLVideoElement>(null);
  const containerRef = useRef<HTMLDivElement>(null);
  const [isPlaying, setIsPlaying] = useState(false);
  const [currentTime, setCurrentTime] = useState(0);
  const [duration, setDuration] = useState(0);
  const [volume, setVolume] = useState(1);
  const [isMuted, setIsMuted] = useState(false);
  const [isLoaded, setIsLoaded] = useState(false);
  const [isFullscreen, setIsFullscreen] = useState(false);
  const [showControls, setShowControls] = useState(true);
  const [hasStarted, setHasStarted] = useState(false);
  const controlsTimeoutRef = useRef<NodeJS.Timeout | null>(null);

  // Handle video events
  useEffect(() => {
    const video = videoRef.current;
    if (!video) return;

    const handleLoadedMetadata = () => {
      setDuration(video.duration);
      setIsLoaded(true);
    };

    const handleTimeUpdate = () => {
      setCurrentTime(video.currentTime);
    };

    const handleEnded = () => {
      setIsPlaying(false);
      setHasStarted(false);
    };

    const handlePlay = () => setIsPlaying(true);
    const handlePause = () => setIsPlaying(false);

    video.addEventListener("loadedmetadata", handleLoadedMetadata);
    video.addEventListener("timeupdate", handleTimeUpdate);
    video.addEventListener("ended", handleEnded);
    video.addEventListener("play", handlePlay);
    video.addEventListener("pause", handlePause);

    // If metadata already loaded (cached video)
    if (video.readyState >= 1) {
      handleLoadedMetadata();
    }

    return () => {
      video.removeEventListener("loadedmetadata", handleLoadedMetadata);
      video.removeEventListener("timeupdate", handleTimeUpdate);
      video.removeEventListener("ended", handleEnded);
      video.removeEventListener("play", handlePlay);
      video.removeEventListener("pause", handlePause);
    };
  }, []);

  // Handle fullscreen changes
  useEffect(() => {
    const handleFullscreenChange = () => {
      setIsFullscreen(!!document.fullscreenElement);
    };

    document.addEventListener("fullscreenchange", handleFullscreenChange);
    return () => {
      document.removeEventListener("fullscreenchange", handleFullscreenChange);
    };
  }, []);

  // Update video volume when volume state changes
  useEffect(() => {
    if (videoRef.current) {
      videoRef.current.volume = isMuted ? 0 : volume;
    }
  }, [volume, isMuted]);

  // Auto-hide controls after inactivity
  const resetControlsTimeout = useCallback(() => {
    if (controlsTimeoutRef.current) {
      clearTimeout(controlsTimeoutRef.current);
    }
    setShowControls(true);
    if (isPlaying) {
      controlsTimeoutRef.current = setTimeout(() => {
        setShowControls(false);
      }, 3000);
    }
  }, [isPlaying]);

  useEffect(() => {
    return () => {
      if (controlsTimeoutRef.current) {
        clearTimeout(controlsTimeoutRef.current);
      }
    };
  }, []);

  const togglePlay = useCallback(() => {
    const video = videoRef.current;
    if (!video) return;

    if (isPlaying) {
      video.pause();
    } else {
      video.play();
      setHasStarted(true);
    }
    resetControlsTimeout();
  }, [isPlaying, resetControlsTimeout]);

  const handleSeek = useCallback((value: number[]) => {
    const video = videoRef.current;
    if (!video || !isLoaded) return;

    const newTime = (value[0] / 100) * duration;
    video.currentTime = newTime;
    setCurrentTime(newTime);
  }, [duration, isLoaded]);

  const handleVolumeChange = useCallback((value: number[]) => {
    setVolume(value[0] / 100);
    if (value[0] > 0) {
      setIsMuted(false);
    }
  }, []);

  const toggleMute = useCallback(() => {
    setIsMuted((prev) => !prev);
  }, []);

  const toggleFullscreen = useCallback(async () => {
    const container = containerRef.current;
    if (!container) return;

    try {
      if (!document.fullscreenElement) {
        await container.requestFullscreen();
      } else {
        await document.exitFullscreen();
      }
    } catch {
      // Fullscreen may not be available in all contexts (e.g., iframe restrictions)
      // Silently ignore as the user can still use the player without fullscreen
    }
  }, []);

  const handleContainerClick = useCallback((e: React.MouseEvent) => {
    // Only toggle play if clicking on the video area, not controls
    const target = e.target as HTMLElement;
    if (target.closest("[data-controls]")) return;
    togglePlay();
  }, [togglePlay]);

  const progress = duration > 0 ? (currentTime / duration) * 100 : 0;

  return (
    // eslint-disable-next-line jsx-a11y/no-static-element-interactions -- Mouse events control auto-hide of video controls, keyboard users can always access controls via tab
    <div
      ref={containerRef}
      className={cn(
        "relative rounded-lg border bg-background/50 overflow-hidden group",
        isFullscreen && "border-0 rounded-none",
        className
      )}
      onMouseMove={resetControlsTimeout}
      onMouseLeave={() => isPlaying && setShowControls(false)}
      data-testid="video-player"
    >
      {/* Filename */}
      {filename && !isFullscreen && (
        <p className="text-xs text-muted-foreground p-2 truncate" title={filename}>
          {filename}
        </p>
      )}

      {/* Video container - click to toggle play (enhancement, not primary interaction method) */}
      {/* eslint-disable-next-line jsx-a11y/click-events-have-key-events, jsx-a11y/no-static-element-interactions -- Clicking video area is enhancement, primary control is the play overlay button */}
      <div
        className={cn(
          "relative bg-black cursor-pointer",
          isFullscreen ? "w-full h-full flex items-center justify-center" : "max-h-[300px]"
        )}
        onClick={handleContainerClick}
      >
        <video
          ref={videoRef}
          src={src}
          preload="metadata"
          className={cn(
            "w-full",
            isFullscreen ? "max-h-full" : "max-h-[300px]"
          )}
          aria-label={filename ? `Video: ${filename}` : "Video player"}
        >
          {type && <source src={src} type={type} />}
          Your browser does not support video playback.
        </video>

        {/* Play overlay (shown before video starts) */}
        {!hasStarted && (
          <div className="absolute inset-0 flex items-center justify-center bg-black/30">
            <button
              type="button"
              onClick={togglePlay}
              className="flex items-center justify-center w-16 h-16 rounded-full bg-primary/90 text-primary-foreground hover:bg-primary transition-colors"
              aria-label="Play video"
            >
              <Play className="h-8 w-8 ml-1" />
            </button>
          </div>
        )}

        {/* Controls overlay */}
        {hasStarted && (
          <div
            data-controls
            className={cn(
              "absolute bottom-0 left-0 right-0 bg-gradient-to-t from-black/80 to-transparent p-3 transition-opacity duration-300",
              showControls ? "opacity-100" : "opacity-0"
            )}
          >
            {/* Progress bar */}
            <div className="mb-2">
              <Slider
                value={[progress]}
                max={100}
                step={0.1}
                onValueChange={handleSeek}
                className="w-full"
                aria-label="Seek video"
              />
            </div>

            {/* Control buttons */}
            <div className="flex items-center gap-2">
              <Button
                variant="ghost"
                size="icon"
                className="h-8 w-8 shrink-0 text-white hover:bg-white/20"
                onClick={togglePlay}
                aria-label={isPlaying ? "Pause" : "Play"}
              >
                {isPlaying ? (
                  <Pause className="h-4 w-4" />
                ) : (
                  <Play className="h-4 w-4" />
                )}
              </Button>

              {/* Volume control */}
              <div className="flex items-center gap-1">
                <Button
                  variant="ghost"
                  size="icon"
                  className="h-8 w-8 shrink-0 text-white hover:bg-white/20"
                  onClick={toggleMute}
                  aria-label={isMuted ? "Unmute" : "Mute"}
                >
                  {isMuted || volume === 0 ? (
                    <VolumeX className="h-4 w-4" />
                  ) : (
                    <Volume2 className="h-4 w-4" />
                  )}
                </Button>
                <Slider
                  value={[isMuted ? 0 : volume * 100]}
                  max={100}
                  step={1}
                  onValueChange={handleVolumeChange}
                  className="w-20"
                  aria-label="Volume"
                />
              </div>

              {/* Time display */}
              <span className="text-xs text-white tabular-nums whitespace-nowrap flex-1">
                {formatTime(currentTime)} / {formatTime(duration)}
              </span>

              {/* Fullscreen button */}
              <Button
                variant="ghost"
                size="icon"
                className="h-8 w-8 shrink-0 text-white hover:bg-white/20"
                onClick={toggleFullscreen}
                aria-label={isFullscreen ? "Exit fullscreen" : "Fullscreen"}
              >
                {isFullscreen ? (
                  <Minimize className="h-4 w-4" />
                ) : (
                  <Maximize className="h-4 w-4" />
                )}
              </Button>
            </div>
          </div>
        )}
      </div>

      {/* Video info (duration and file size) */}
      {!isFullscreen && (
        <div className="flex items-center gap-2 px-2 py-1 text-xs text-muted-foreground">
          {isLoaded && <span>{formatTime(duration)}</span>}
          {isLoaded && fileSize && <span>•</span>}
          {fileSize && <span>{formatFileSize(fileSize)}</span>}
        </div>
      )}
    </div>
  );
}
