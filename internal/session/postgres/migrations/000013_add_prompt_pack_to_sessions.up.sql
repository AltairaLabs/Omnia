-- Add prompt_pack_name and prompt_pack_version columns to sessions table.
-- These columns propagate the PromptPack identity from the agent facade so
-- that the eval worker can load eval definitions for non-PromptKit agents.
ALTER TABLE sessions ADD COLUMN IF NOT EXISTS prompt_pack_name TEXT;
ALTER TABLE sessions ADD COLUMN IF NOT EXISTS prompt_pack_version TEXT;
