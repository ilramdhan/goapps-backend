-- 000419: Seed mst_product_grade from Oracle CSV (CST_MST_PRODUCT_GRADE).
-- Generated: 2026-06-26  Source rows: 40
-- No new schema columns needed — all fields already exist.
-- pg_code derived as 'GRD-' + CMPG_SYS_ID for guaranteed uniqueness.
-- CMPG_POY_B_C_GRADE → pg_name + pg_grade_label.
-- CMPG_DETAIL_PRODUCT → pg_detail_product + pg_description.
-- bc_perc/non_std_perc/bc_recovery_rate default to 0/0/80 (not in Oracle CSV).

BEGIN;

DELETE FROM public.mst_product_grade;

INSERT INTO public.mst_product_grade (
  pg_oracle_sys_id, pg_code, pg_name, pg_description,
  pg_detail_product, pg_grade_label,
  std_selling_price, loss_pct, sp_value, seq_no,
  bc_perc, non_std_perc, bc_recovery_rate,
  is_active, created_by, created_at, updated_by, updated_at
)
VALUES
  ('20201001', 'GRD-20201001', 'Type 1POY BC', 'PTY 150/300 NI/HIM all white', 'PTY 150/300 NI/HIM all white', 'Type 1POY BC', 1.25, 0.5, 0.75, 1, 0, 0, 80, TRUE, 'ADMIN', '2020-10-19 08:55:38', 'SINTIA', '2025-11-20 15:13:38'),
  ('20201002', 'GRD-20201002', 'Type 2POY BC', 'SD,BR,FR,RCL,AMB,PES,OBR,Melange Rw', 'SD,BR,FR,RCL,AMB,PES,OBR,Melange Rw', 'Type 2POY BC', 1.25, 0.5, 0.75, 2, 0, 0, 80, TRUE, 'ADMIN', '2020-10-19 08:56:17', 'SINTIA', '2025-11-20 15:13:38'),
  ('20201003', 'GRD-20201003', 'Type 3POY BC', 'BSD/BBR/OSD', 'BSD/BBR/OSD', 'Type 3POY BC', 1.25, 0.45, 0.8, 3, 0, 0, 80, TRUE, 'ADMIN', '2020-10-19 08:56:17', 'SINTIA', '2025-11-20 15:13:38'),
  ('20201004', 'GRD-20201004', 'Type 4POY BC', 'DSD/DFD/Lumino', 'DSD/DFD/Lumino', 'Type 4POY BC', 1.25, 0.45, 0.8, 4, 0, 0, 80, TRUE, 'ADMIN', '2020-10-19 08:56:18', 'SINTIA', '2025-11-20 15:13:38'),
  ('20201005', 'GRD-20201005', 'Type 5POY BC', 'Melange (Raw White,FR,FSD,BR - Black)', 'Melange (Raw White,FR,FSD,BR - Black)', 'Type 5POY BC', 1.25, 0.5, 0.75, 5, 0, 0, 80, TRUE, 'ADMIN', '2020-10-19 08:56:18', 'SINTIA', '2025-11-20 15:13:38'),
  ('20201006', 'GRD-20201006', 'Type 6POY BC', 'Melange (Color - Black)', 'Melange (Color - Black)', 'Type 6POY BC', 1.25, 0.45, 0.8, 6, 0, 0, 80, TRUE, 'ADMIN', '2020-10-19 08:56:18', 'SINTIA', '2025-11-20 15:13:38'),
  ('20201007', 'GRD-20201007', 'Type 7POY BC', 'DBR <=600D', 'DBR <=600D', 'Type 7POY BC', 1.25, 0.45, 0.8, 7, 0, 0, 80, TRUE, 'ADMIN', '2020-10-19 08:56:18', 'SINTIA', '2025-11-20 15:13:38'),
  ('20201008', 'GRD-20201008', 'Type 8POY BC', 'DBR >600D', 'DBR >600D', 'Type 8POY BC', 1.25, 0.45, 0.8, 8, 0, 0, 80, TRUE, 'ADMIN', '2020-10-19 08:56:18', 'SINTIA', '2025-11-20 15:13:38'),
  ('20201009', 'GRD-20201009', 'Type 9POY BC', 'ACY', 'ACY', 'Type 9POY BC', NULL, NULL, NULL, 9, 0, 0, 80, TRUE, 'ADMIN', '2020-10-19 08:56:58', NULL, NULL),
  ('20201010', 'GRD-20201010', 'Type 10POY BC', 'ITY', 'ITY', 'Type 10POY BC', NULL, NULL, NULL, 10, 0, 0, 80, TRUE, 'ADMIN', '2020-10-19 08:56:58', NULL, NULL),
  ('20201011', 'GRD-20201011', 'Type 11POY BC', 'Rewinding', 'Rewinding', 'Type 11POY BC', 1.25, 0.45, 0.8, 11, 0, 0, 80, TRUE, 'ADMIN', '2020-10-19 08:56:58', 'SINTIA', '2025-11-20 15:13:38'),
  ('20201012', 'GRD-20201012', 'Type 12POY BC', 'PTY 150/300 NI/HIM black product', 'PTY 150/300 NI/HIM black product', 'Type 12POY BC', 1.35, 0.5, 0.85, 12, 0, 0, 80, TRUE, 'ADMIN', '2020-10-19 08:56:58', 'SINTIA', '2025-11-20 15:16:00'),
  ('20201013', 'GRD-20201013', 'Type 1 NS', 'PTY 150/300 NI/HIM all white', 'PTY 150/300 NI/HIM all white', 'Type 1 NS', NULL, 0.05, 0.05, 13, 0, 0, 80, TRUE, 'ADMIN', '2020-10-19 08:57:45', NULL, NULL),
  ('20201014', 'GRD-20201014', 'Type 2 NS', 'SD,BR,FR,RCL,AMB,PES,OBR,Melange Rw', 'SD,BR,FR,RCL,AMB,PES,OBR,Melange Rw', 'Type 2 NS', NULL, 0.05, -0.05, 14, 0, 0, 80, TRUE, 'ADMIN', '2020-10-19 08:57:45', 'SINTIA', '2023-11-09 09:35:37'),
  ('20201015', 'GRD-20201015', 'Type 3 NS', 'BSD/BBR/OSD', 'BSD/BBR/OSD', 'Type 3 NS', NULL, 0.05, NULL, 15, 0, 0, 80, TRUE, 'ADMIN', '2020-10-19 08:57:45', NULL, NULL),
  ('20201016', 'GRD-20201016', 'Type 4 NS', 'DSD/DFD/Lumino', 'DSD/DFD/Lumino', 'Type 4 NS', NULL, 0, NULL, 16, 0, 0, 80, TRUE, 'ADMIN', '2020-10-19 08:57:45', NULL, NULL),
  ('20201017', 'GRD-20201017', 'Type 5 NS', 'Melange (Raw White,FR,FSD,BR - Black)', 'Melange (Raw White,FR,FSD,BR - Black)', 'Type 5 NS', NULL, 0.05, 0.05, 17, 0, 0, 80, TRUE, 'ADMIN', '2020-10-19 08:57:45', NULL, NULL),
  ('20201018', 'GRD-20201018', 'Type 6 NS', 'Melange (Color - Black)', 'Melange (Color - Black)', 'Type 6 NS', NULL, 0, NULL, 18, 0, 0, 80, TRUE, 'ADMIN', '2020-10-19 08:57:45', NULL, NULL),
  ('20201019', 'GRD-20201019', 'Type 7 NS', 'DBR <=600D', 'DBR <=600D', 'Type 7 NS', NULL, 0, NULL, 19, 0, 0, 80, TRUE, 'ADMIN', '2020-10-19 08:57:46', NULL, NULL),
  ('20201020', 'GRD-20201020', 'Type 8 NS', 'DBR >600D', 'DBR >600D', 'Type 8 NS', NULL, 0, NULL, 20, 0, 0, 80, TRUE, 'ADMIN', '2020-10-19 08:57:46', NULL, NULL),
  ('20201021', 'GRD-20201021', 'Type 9 NS', 'ACY', 'ACY', 'Type 9 NS', NULL, 0, NULL, 21, 0, 0, 80, TRUE, 'ADMIN', '2020-10-19 08:57:46', NULL, NULL),
  ('20201022', 'GRD-20201022', 'Type 10 NS', 'ITY', 'ITY', 'Type 10 NS', NULL, 0.28, -0.28, 22, 0, 0, 80, TRUE, 'ADMIN', '2020-10-19 08:57:46', 'SINTIA', '2023-06-02 14:36:47'),
  ('20201023', 'GRD-20201023', 'Type 11 NS', 'Rewinding', 'Rewinding', 'Type 11 NS', NULL, 0, NULL, 23, 0, 0, 80, TRUE, 'ADMIN', '2020-10-19 08:57:46', NULL, NULL),
  ('20201024', 'GRD-20201024', 'Type 12 NS', 'PTY 150/300 NI/HIM black product', 'PTY 150/300 NI/HIM black product', 'Type 12 NS', NULL, 0.05, NULL, 24, 0, 0, 80, TRUE, 'ADMIN', '2020-10-19 08:57:46', NULL, NULL),
  ('20201025', 'GRD-20201025', 'Type 1 BC', 'PTY 150/300 NI/HIM all white', 'PTY 150/300 NI/HIM all white', 'Type 1 BC', 1.25, 0.1, 1.15, 25, 0, 0, 80, TRUE, 'ADMIN', '2020-10-19 08:58:28', 'SINTIA', '2025-11-20 15:13:38'),
  ('20201026', 'GRD-20201026', 'Type 2 BC', 'SD,BR,FR,RCL,AMB,PES,OBR,Melange Rw', 'SD,BR,FR,RCL,AMB,PES,OBR,Melange Rw', 'Type 2 BC', 1.25, 0.25, 1, 26, 0, 0, 80, TRUE, 'ADMIN', '2020-10-19 08:58:28', 'SINTIA', '2025-11-20 15:13:38'),
  ('20201027', 'GRD-20201027', 'Type 3 BC', 'BSD/BBR/OSD', 'BSD/BBR/OSD', 'Type 3 BC', 1.25, 0.35, 0.9, 27, 0, 0, 80, TRUE, 'ADMIN', '2020-10-19 08:58:28', 'SINTIA', '2025-11-20 15:13:38'),
  ('20201028', 'GRD-20201028', 'Type 4 BC', 'DSD/DFD/Lumino', 'DSD/DFD/Lumino', 'Type 4 BC', 1.25, 0.15, 1.1, 28, 0, 0, 80, TRUE, 'ADMIN', '2020-10-19 08:58:28', 'SINTIA', '2025-11-20 15:13:38'),
  ('20201029', 'GRD-20201029', 'Type 5 BC', 'Melange (Raw White,FR,FSD,BR - Black)', 'Melange (Raw White,FR,FSD,BR - Black)', 'Type 5 BC', 1.25, 0.4, 0.85, 29, 0, 0, 80, TRUE, 'ADMIN', '2020-10-19 08:58:28', 'SINTIA', '2025-11-20 15:13:38'),
  ('20201030', 'GRD-20201030', 'Type 6 BC', 'Melange (Color - Black)', 'Melange (Color - Black)', 'Type 6 BC', 1.25, 0.4, 0.85, 30, 0, 0, 80, TRUE, 'ADMIN', '2020-10-19 08:58:28', 'SINTIA', '2025-11-20 15:13:38'),
  ('20201031', 'GRD-20201031', 'Type 7 BC', 'DBR <=600D', 'DBR <=600D', 'Type 7 BC', 1.25, 0.3, 0.95, 31, 0, 0, 80, TRUE, 'ADMIN', '2020-10-19 08:58:28', 'SINTIA', '2025-11-20 15:13:38'),
  ('20201032', 'GRD-20201032', 'Type 8 BC', 'DBR >600D', 'DBR >600D', 'Type 8 BC', 1.25, 0.6, 0.65, 32, 0, 0, 80, TRUE, 'ADMIN', '2020-10-19 08:58:28', 'SINTIA', '2025-11-20 15:13:38'),
  ('20201033', 'GRD-20201033', 'Type 9 BC', 'ACY', 'ACY', 'Type 9 BC', 1.25, 0.5, 0.75, 33, 0, 0, 80, TRUE, 'ADMIN', '2020-10-19 08:58:28', 'SINTIA', '2025-11-20 15:13:38'),
  ('20201034', 'GRD-20201034', 'Type 10 BC', 'ITY', 'ITY', 'Type 10 BC', 1.42, 0.63, 0.79, 34, 0, 0, 80, TRUE, 'ADMIN', '2020-10-19 08:58:28', 'SINTIA', '2025-11-20 15:17:30'),
  ('20201035', 'GRD-20201035', 'Type 11 BC', 'Rewinding', 'Rewinding', 'Type 11 BC', 1.25, 0.4, 0.85, 35, 0, 0, 80, TRUE, 'ADMIN', '2020-10-19 08:58:28', 'SINTIA', '2025-11-20 15:13:38'),
  ('20201036', 'GRD-20201036', 'Type 12 BC', 'PTY 150/300 NI/HIM black product', 'PTY 150/300 NI/HIM black product', 'Type 12 BC', 1.35, 0.1, 1.25, 36, 0, 0, 80, TRUE, 'ADMIN', '2020-10-19 08:58:28', 'SINTIA', '2025-11-20 15:16:00'),
  ('20230339', 'GRD-20230339', 'Type Spcl (30)', 'Special Product (BC)', 'Special Product (BC)', 'Type Spcl (30)', 0.3, NULL, 0.3, NULL, 0, 0, 80, TRUE, 'SINTIA', '2023-03-10 14:17:11', 'SINTIA', '2023-03-17 10:36:06'),
  ('20230340', 'GRD-20230340', 'Type Spcl 1 (70)', 'Special Product (BC)', 'Special Product (BC)', 'Type Spcl 1 (70)', 0.7, NULL, 0.7, NULL, 0, 0, 80, TRUE, 'SINTIA', '2023-03-17 10:36:48', 'SINTIA', '2023-07-28 13:20:07'),
  ('20230641', 'GRD-20230641', 'Type Spcl 2 ($1)', 'Special Product (BC)', 'Special Product (BC)', 'Type Spcl 2 ($1)', 1, NULL, 1, NULL, 0, 0, 80, TRUE, 'SINTIA', '2023-06-09 16:27:11', NULL, NULL),
  ('20231042', 'GRD-20231042', 'Type Spcl ($1.25)', 'Special Product (BC)', 'Special Product (BC)', 'Type Spcl ($1.25)', 1.25, NULL, 1.25, NULL, 0, 0, 80, TRUE, 'SINTIA', '2023-10-13 11:11:02', 'SINTIA', '2026-04-11 09:20:59');

DO $$
DECLARE v_inserted INTEGER;
BEGIN
  SELECT COUNT(*) INTO v_inserted FROM public.mst_product_grade WHERE deleted_at IS NULL;
  RAISE NOTICE '000419: mst_product_grade inserted=%  (expected=40)', v_inserted;
END $$;

COMMIT;