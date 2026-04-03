-- ============================================================
-- LINE-OF-SIGHT (LOS) COVERAGE QUERY
-- Output: single polygon connecting the last visible pixel
-- coordinate of each ray around the full 360° sweep.
-- ============================================================

WITH

-- ------------------------------------------------------------
-- STEP 1: INPUT PARAMETERS
-- ------------------------------------------------------------
params AS (
  SELECT
    106.7  AS lon,
    10.8   AS lat,
    50.0   AS min_radius_m,
    500.0  AS max_radius_m,
    30.0   AS height_threshold,
    720    AS ray_count
),

-- ------------------------------------------------------------
-- STEP 2: CENTER POINT
-- Computed once, reused everywhere via cast.
-- ------------------------------------------------------------
center AS (
  SELECT ST_SetSRID(ST_MakePoint(p.lon, p.lat), 4326) AS geom
  FROM params p
),

-- ------------------------------------------------------------
-- STEP 3: ANNULUS
-- Active zone between min and max radius.
-- Only pixels inside this ring are processed.
-- ------------------------------------------------------------
annulus AS (
  SELECT
    ST_Difference(
      ST_SetSRID(ST_Buffer(c.geom::geography, p.max_radius_m)::geometry, 4326),
      ST_SetSRID(ST_Buffer(c.geom::geography, p.min_radius_m)::geometry, 4326)
    ) AS geom
  FROM center c, params p
),

-- ------------------------------------------------------------
-- STEP 4: RAW PIXEL DATA
-- Clips raster to annulus first to minimize pixel expansion.
-- Only computes coordinates, distance, and azimuth per pixel.
-- No geometry objects created per pixel.
-- ------------------------------------------------------------
raw_pixels AS (
  SELECT
    ST_RasterToWorldCoordX(clipped.rast, x, y)   AS px_lon,
    ST_RasterToWorldCoordY(clipped.rast, x, y)   AS px_lat,
    ST_Value(clipped.rast, x, y)                  AS terrain_height,

    sqrt(
      pow((ST_RasterToWorldCoordX(clipped.rast, x, y) - p.lon) * 111320.0 * cos(radians(p.lat)), 2) +
      pow((ST_RasterToWorldCoordY(clipped.rast, x, y) - p.lat) * 110540.0, 2)
    )                                              AS dist_m,

    atan2(
      (ST_RasterToWorldCoordX(clipped.rast, x, y) - p.lon) * cos(radians(p.lat)),
       ST_RasterToWorldCoordY(clipped.rast, x, y) - p.lat
    )                                              AS azimuth_rad

  FROM (
    SELECT ST_Clip(r.rast, a.geom, true) AS rast
    FROM raster_table r
    JOIN annulus a ON ST_Intersects(r.rast, a.geom)
  ) clipped
  CROSS JOIN params p
  CROSS JOIN generate_series(1, ST_Width(clipped.rast))  AS x
  CROSS JOIN generate_series(1, ST_Height(clipped.rast)) AS y
  WHERE ST_Value(clipped.rast, x, y) IS NOT NULL
),

-- ------------------------------------------------------------
-- STEP 5: FIRST BLOCKER PER RAY
-- Filters to only blocker pixels (terrain >= height_threshold),
-- then picks the closest one per ray using DISTINCT ON.
-- Rays absent here = no blocker = visible to max_radius.
-- ------------------------------------------------------------
first_blocker_per_ray AS (
  SELECT DISTINCT ON (ray)
    floor(
      degrees(
        CASE
          WHEN azimuth_rad < 0 THEN azimuth_rad + 2 * pi()
          ELSE azimuth_rad
        END
      ) * p.ray_count / 360.0
    )::int   AS ray,
    dist_m   AS block_dist_m

  FROM raw_pixels
  CROSS JOIN params p
  WHERE terrain_height >= p.height_threshold
  ORDER BY ray, dist_m  -- closest blocker per ray
),

-- ------------------------------------------------------------
-- STEP 6: LAST VISIBLE PIXEL PER RAY  (replaces old steps 6+7)
-- For each ray, finds the FURTHEST pixel that is:
--   a) below height_threshold (not a blocker itself)
--   b) closer than the first blocker on that ray
--      (if no blocker exists, any pixel up to max_radius qualifies)
--
-- DISTINCT ON (ray) + ORDER BY ray, dist_m DESC means:
--   → pixels sorted by ray then distance descending
--   → first row per ray = the FURTHEST visible pixel on that ray
--   → this coordinate becomes the ray's endpoint on the polygon
-- ------------------------------------------------------------
last_visible_per_ray AS (
  SELECT DISTINCT ON (ray)
    floor(
      degrees(
        CASE
          WHEN rp.azimuth_rad < 0 THEN rp.azimuth_rad + 2 * pi()
          ELSE rp.azimuth_rad
        END
      ) * p.ray_count / 360.0
    )::int                                            AS ray,
    rp.px_lon,
    rp.px_lat,
    rp.dist_m

  FROM raw_pixels rp
  CROSS JOIN params p
  LEFT JOIN first_blocker_per_ray b
    ON floor(
         degrees(
           CASE
             WHEN rp.azimuth_rad < 0 THEN rp.azimuth_rad + 2 * pi()
             ELSE rp.azimuth_rad
           END
         ) * p.ray_count / 360.0
       )::int = b.ray

  WHERE rp.terrain_height < p.height_threshold        -- must not be a blocker
    AND (
      b.block_dist_m IS NULL                           -- no blocker on this ray
      OR rp.dist_m < b.block_dist_m                   -- or pixel is before the blocker
    )

  ORDER BY ray, rp.dist_m DESC  -- DESC = furthest visible pixel first
)

-- ------------------------------------------------------------
-- FINAL OUTPUT: CONNECT ALL RAY ENDPOINTS INTO ONE POLYGON
-- Orders all ray endpoint coordinates by ray index (0..ray_count-1)
-- builds a linestring through them, then closes it into a polygon.
--
-- ST_AddPoint(..., ST_StartPoint(...)) closes the ring by
-- appending the first point at the end — required by ST_MakePolygon.
--
-- The result is a single polygon whose boundary traces the
-- furthest visible point of each ray around the full 360° sweep.
-- ------------------------------------------------------------
SELECT
  ST_MakePolygon(
    ST_AddPoint(
      line,
      ST_StartPoint(line)   -- close the ring back to first ray endpoint
    )
  ) AS los_coverage_polygon
FROM (
  SELECT
    ST_MakeLine(
      array_agg(
        ST_SetSRID(ST_MakePoint(px_lon, px_lat), 4326)
        ORDER BY ray    -- must be ordered by angle to form a valid ring
      )
    ) AS line
  FROM last_visible_per_ray
) ring;
