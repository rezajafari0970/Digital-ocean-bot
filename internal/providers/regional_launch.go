package providers

import "fmt"

// FreezeRegionalLaunches binds semantic catalog choices to exact native IDs.
// The result is persisted with the deployment and never re-resolved on retry.
// Existing global-ID drivers need no mapping and preserve their current behavior.
func FreezeRegionalLaunches(c Catalog, planID, imageID string, regions []string) (map[string]RegionalLaunch, []string, error) {
	var planMap, imageMap map[string]string
	planFound, imageFound := false, false
	for _, p := range c.Plans {
		if p.ID == planID {
			planMap = p.IDsByRegion
			planFound = true
			break
		}
	}
	for _, im := range c.Images {
		if im.ID == imageID {
			imageMap = im.IDsByRegion
			imageFound = true
			break
		}
	}
	if planMap == nil && imageMap == nil {
		return nil, regions, nil
	}
	if !planFound || !imageFound {
		return nil, nil, fmt.Errorf("regional catalog selection missing")
	}
	out := map[string]RegionalLaunch{}
	compatible := []string{}
	for _, r := range regions {
		p, im := planID, imageID
		if planMap != nil {
			p = planMap[r]
		}
		if imageMap != nil {
			im = imageMap[r]
		}
		if p == "" || im == "" {
			continue
		}
		if _, ok := out[r]; ok {
			continue
		}
		out[r] = RegionalLaunch{PlanID: p, ImageID: im}
		compatible = append(compatible, r)
	}
	if len(compatible) == 0 {
		return nil, nil, fmt.Errorf("no compatible regional image and plan")
	}
	return out, compatible, nil
}
