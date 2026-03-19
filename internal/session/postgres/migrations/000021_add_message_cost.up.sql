-- Add cost_usd column to messages table for per-message cost tracking.
-- Session-level estimated_cost_usd is now derived from message costs via AppendMessage.
ALTER TABLE messages ADD COLUMN IF NOT EXISTS cost_usd NUMERIC(12,6) NOT NULL DEFAULT 0;
