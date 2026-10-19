-- +goose Up
-- Video project settings and production outputs (Agent 9, Video Studio).
-- The project id is the content_assets id (asset_type = 'video'); scenes
-- stay in video_scenes (0006).
CREATE TABLE video_projects (
    asset_id UUID PRIMARY KEY REFERENCES content_assets(id),
    brand_id UUID NOT NULL REFERENCES brands(id),
    campaign_id UUID REFERENCES campaigns(id),
    title TEXT NOT NULL,
    prompt TEXT,
    script TEXT,
    settings JSONB NOT NULL,                  -- preset, size, duration, source, voiceover, subtitles, music
    current_job_id UUID REFERENCES ai_jobs(id),
    voice_json JSONB,                         -- {fingerprint, segments[]} of the last voice-over
    subtitle_json JSONB,                      -- {fingerprint, srt_file_id, ass_file_id, cue_count}
    output_json JSONB,                        -- {file_id, thumbnail_file_id, subtitle_file_id, duration_ms, version_no}
    created_by UUID NOT NULL REFERENCES users(id),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_video_projects_campaign ON video_projects (campaign_id, created_at DESC);
CREATE INDEX idx_video_projects_created_by ON video_projects (created_by, created_at DESC);
CREATE TRIGGER trg_video_projects_updated BEFORE UPDATE ON video_projects FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- +goose Down
DROP TABLE IF EXISTS video_projects;
