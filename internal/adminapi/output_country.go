package adminapi

// Resolve the actual resource's observed region before the requested build
// region. Metadata is read in the same statement snapshot as output eligibility.
// Materializing per droplet avoids doing inventory/catalog work per client.
// Catalog data is descriptive only: it never relaxes publication health guards.
const outputCountryLocationsSQL = `WITH output_locations AS MATERIALIZED (
 SELECT dr.id AS droplet_id, a.provider,
 COALESCE(observed.region_id,dr.profile->>'Region','') AS region_id,
 COALESCE(region.meta->>'Name','') AS region_name,
 COALESCE(region.meta->>'CountryCode','') AS country_code
 FROM droplets dr JOIN accounts a ON a.id=dr.account_id
 LEFT JOIN LATERAL (
  SELECT canonical FROM provider_snapshots
  WHERE account_id=dr.account_id AND canonical IS NOT NULL
  ORDER BY created_at DESC LIMIT 1
 ) ps ON true
 LEFT JOIN LATERAL (
  SELECT NULLIF(server->>'RegionID','') AS region_id
  FROM jsonb_array_elements(CASE WHEN jsonb_typeof(ps.canonical->'Inventory'->'Servers')='array'
    THEN ps.canonical->'Inventory'->'Servers' ELSE '[]'::jsonb END) server
  WHERE server->>'ID'=dr.provider_resource_id LIMIT 1
 ) observed ON true
 LEFT JOIN LATERAL (
  SELECT meta FROM jsonb_array_elements(CASE WHEN jsonb_typeof(ps.canonical->'Catalog'->'Regions')='array'
    THEN ps.canonical->'Catalog'->'Regions' ELSE '[]'::jsonb END) meta
  WHERE meta->>'ID'=COALESCE(observed.region_id,dr.profile->>'Region','') LIMIT 1
 ) region ON true
 WHERE dr.state IN ('READY','EXPIRING')
) `
