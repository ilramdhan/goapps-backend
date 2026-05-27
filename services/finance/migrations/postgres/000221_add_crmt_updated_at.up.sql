-- Add missing crmt_updated_at column referenced by cost_rm_type_repository.
ALTER TABLE cost_rm_type
    ADD COLUMN IF NOT EXISTS crmt_updated_at TIMESTAMP WITH TIME ZONE;

UPDATE cost_rm_type SET crmt_updated_at = crmt_created_at WHERE crmt_updated_at IS NULL;

ALTER TABLE cost_rm_type
    ALTER COLUMN crmt_updated_at SET NOT NULL,
    ALTER COLUMN crmt_updated_at SET DEFAULT now();
