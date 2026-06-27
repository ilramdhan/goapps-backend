ALTER TABLE cost_import_job
    DROP CONSTRAINT IF EXISTS chk_cij_entity,
    ADD CONSTRAINT chk_cij_entity CHECK (
        cij_entity IN (
            'product_type',
            'parameter',
            'product_master',
            'capp',
            'cpp',
            'bulk_product_routing',
            'bulk_product_routing_export',
            'bulk_params_only'
        )
    );
