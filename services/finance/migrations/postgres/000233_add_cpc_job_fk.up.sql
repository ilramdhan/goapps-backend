DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint
        WHERE conname = 'fk_cpc_job'
          AND conrelid = 'cst_product_cost'::regclass
    ) THEN
        ALTER TABLE cst_product_cost
          ADD CONSTRAINT fk_cpc_job FOREIGN KEY (cpc_job_id) REFERENCES cal_job(cj_job_id);
    END IF;
END
$$;
