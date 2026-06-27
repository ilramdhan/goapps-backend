-- 000419 DOWN: Remove Oracle CSV seed data for mst_product_grade.
BEGIN;
DELETE FROM public.mst_product_grade;
COMMIT;
