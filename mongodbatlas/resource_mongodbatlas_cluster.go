package mongodbatlas

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"reflect"
	"time"

	"strconv"
	"strings"

	"github.com/hashicorp/terraform/helper/resource"
	"github.com/hashicorp/terraform/helper/schema"

	"github.com/mwielbut/pointy"
	"github.com/spf13/cast"

	matlas "github.com/mongodb/go-client-mongodb-atlas/mongodbatlas"
)

const (
	errorCreate = "error creating MongoDB Cluster: %s"
	errorRead   = "error reading MongoDB Cluster (%s): %s"
	errorDelete = "error deleting MongoDB Cluster (%s): %s"
	errorUpdate = "error updating MongoDB Cluster (%s): %s"
)

func resourceMongoDBAtlasCluster() *schema.Resource {
	return &schema.Resource{
		Create: resourceMongoDBAtlasClusterCreate,
		Read:   resourceMongoDBAtlasClusterRead,
		Update: resourceMongoDBAtlasClusterUpdate,
		Delete: resourceMongoDBAtlasClusterDelete,
		Importer: &schema.ResourceImporter{
			State: resourceMongoDBAtlasClusterImportState,
		},
		Schema: map[string]*schema.Schema{
			"project_id": {
				Type:     schema.TypeString,
				Required: true,
				ForceNew: true,
			},
			"cluster_id": {
				Type:     schema.TypeString,
				Computed: true,
			},
			"auto_scaling_disk_gb_enabled": {
				Type:     schema.TypeBool,
				Optional: true,
				Default:  true,
			},
			"backup_enabled": {
				Type:     schema.TypeBool,
				Optional: true,
				Default:  false,
			},
			"bi_connector": {
				Type:     schema.TypeMap,
				Optional: true,
				Computed: true,
				Elem: &schema.Resource{
					Schema: map[string]*schema.Schema{
						"enabled": {
							Type:     schema.TypeString,
							Optional: true,
							Computed: true,
						},
						"read_preference": {
							Type:     schema.TypeString,
							Optional: true,
							Computed: true,
						},
					},
				},
			},
			"cluster_type": {
				Type:     schema.TypeString,
				Optional: true,
				Computed: true,
			},
			"disk_size_gb": {
				Type:     schema.TypeFloat,
				Optional: true,
				Computed: true,
			},
			"encryption_at_rest_provider": {
				Type:     schema.TypeString,
				Optional: true,
				Computed: true,
			},
			"name": {
				Type:     schema.TypeString,
				Required: true,
			},
			"mongo_db_major_version": {
				Type:     schema.TypeString,
				Optional: true,
				Computed: true,
			},
			"num_shards": {
				Type:     schema.TypeInt,
				Optional: true,
				Default:  1,
			},
			"provider_backup_enabled": {
				Type:     schema.TypeBool,
				Optional: true,
				Default:  false,
			},
			"provider_instance_size_name": {
				Type:     schema.TypeString,
				Required: true,
			},
			"provider_name": {
				Type:     schema.TypeString,
				Required: true,
			},
			"backing_provider_name": {
				Type:     schema.TypeString,
				Optional: true,
				Computed: true,
			},
			"provider_disk_iops": {
				Type:     schema.TypeInt,
				Optional: true,
				Computed: true,
			},
			"provider_disk_type_name": {
				Type:     schema.TypeString,
				Optional: true,
				Computed: true,
			},
			"provider_encrypt_ebs_volume": {
				Type:     schema.TypeBool,
				Optional: true,
				Computed: true,
			},
			"provider_region_name": {
				Type:     schema.TypeString,
				Optional: true,
				Computed: true,
			},
			"provider_volume_type": {
				Type:     schema.TypeString,
				Optional: true,
				Computed: true,
			},
			"replication_factor": {
				Type:     schema.TypeInt,
				Optional: true,
				Computed: true,
			},
			"replication_specs": {
				Type:     schema.TypeList,
				Optional: true,
				Computed: true,
				Elem: &schema.Resource{
					Schema: map[string]*schema.Schema{
						"id": {
							Type:     schema.TypeString,
							Optional: true,
							Computed: true,
						},
						"num_shards": {
							Type:     schema.TypeInt,
							Required: true,
						},
						"regions_config": {
							Type:     schema.TypeSet,
							Optional: true,
							Computed: true,
							Elem: &schema.Resource{
								Schema: map[string]*schema.Schema{
									"region_name": {
										Type:     schema.TypeString,
										Optional: true,
										Computed: true,
									},
									"electable_nodes": {
										Type:     schema.TypeInt,
										Optional: true,
										Computed: true,
									},
									"priority": {
										Type:     schema.TypeInt,
										Optional: true,
										Computed: true,
									},
									"read_only_nodes": {
										Type:     schema.TypeInt,
										Optional: true,
										Default:  0,
									},
									"analytics_nodes": {
										Type:     schema.TypeInt,
										Optional: true,
										Default:  0,
									},
								},
							},
						},
						"zone_name": {
							Type:     schema.TypeString,
							Optional: true,
							Default:  "ZoneName managed by Terraform",
						},
					},
				},
			},
			"mongo_db_version": {
				Type:     schema.TypeString,
				Computed: true,
			},
			"mongo_uri": {
				Type:     schema.TypeString,
				Computed: true,
			},
			"mongo_uri_updated": {
				Type:     schema.TypeString,
				Computed: true,
			},
			"mongo_uri_with_options": {
				Type:     schema.TypeString,
				Computed: true,
			},
			"paused": {
				Type:     schema.TypeBool,
				Computed: true,
			},
			"srv_address": {
				Type:     schema.TypeString,
				Computed: true,
			},
			"state_name": {
				Type:     schema.TypeString,
				Computed: true,
			},
		},
	}
}

func resourceMongoDBAtlasClusterCreate(d *schema.ResourceData, meta interface{}) error {
	//Get client connection.
	conn := meta.(*matlas.Client)
	projectID := d.Get("project_id").(string)

	//validate cluster_type conditional
	if _, ok := d.GetOk("replication_specs"); ok {
		if _, ok1 := d.GetOk("cluster_type"); !ok1 {
			return fmt.Errorf("`cluster_type` should be set when `replication_specs` is set")
		}

		if _, ok1 := d.GetOk("num_shards"); !ok1 {
			return fmt.Errorf("`num_shards` should be set when `replication_specs` is set")
		}
	}

	biConnector, err := expandBiConnector(d)
	if err != nil {
		return fmt.Errorf(errorCreate, err)
	}

	providerSettings := expandProviderSetting(d)

	replicationSpecs, err := expandReplicationSpecs(d)

	if err != nil {
		return fmt.Errorf(errorCreate, err)
	}

	autoScaling := matlas.AutoScaling{
		DiskGBEnabled: pointy.Bool(d.Get("auto_scaling_disk_gb_enabled").(bool)),
	}

	clusterRequest := &matlas.Cluster{
		Name:                     d.Get("name").(string),
		EncryptionAtRestProvider: d.Get("encryption_at_rest_provider").(string),
		MongoDBMajorVersion:      d.Get("mongo_db_major_version").(string),
		ClusterType:              cast.ToString(d.Get("cluster_type")),
		BackupEnabled:            pointy.Bool(d.Get("backup_enabled").(bool)),
		DiskSizeGB:               pointy.Float64(d.Get("disk_size_gb").(float64)),
		ProviderBackupEnabled:    pointy.Bool(d.Get("provider_backup_enabled").(bool)),
		AutoScaling:              autoScaling,
		BiConnector:              biConnector,
		ProviderSettings:         &providerSettings,
		ReplicationSpecs:         replicationSpecs,
	}

	if r, ok := d.GetOk("replication_factor"); ok {
		clusterRequest.ReplicationFactor = pointy.Int64(cast.ToInt64(r))
	}

	if n, ok := d.GetOk("num_shards"); ok {
		clusterRequest.NumShards = pointy.Int64(cast.ToInt64(n))
	}

	cluster, _, err := conn.Clusters.Create(context.Background(), projectID, clusterRequest)
	if err != nil {
		return fmt.Errorf(errorCreate, err)
	}

	stateConf := &resource.StateChangeConf{
		Pending:    []string{"CREATING", "UPDATING", "REPAIRING", "REPEATING"},
		Target:     []string{"IDLE"},
		Refresh:    resourceClusterRefreshFunc(d.Get("name").(string), projectID, conn),
		Timeout:    d.Timeout(schema.TimeoutCreate),
		MinTimeout: 30 * time.Second,
		Delay:      1 * time.Minute,
	}

	// Wait, catching any errors
	_, err = stateConf.WaitForState()
	if err != nil {
		return fmt.Errorf(errorCreate, err)
	}

	d.SetId(encodeStateID(map[string]string{
		"cluster_id":   cluster.ID,
		"project_id":   projectID,
		"cluster_name": cluster.Name,
	}))

	return resourceMongoDBAtlasClusterRead(d, meta)
}

func resourceMongoDBAtlasClusterRead(d *schema.ResourceData, meta interface{}) error {
	//Get client connection.
	conn := meta.(*matlas.Client)
	ids := decodeStateID(d.Id())
	projectID := ids["project_id"]
	clusterName := ids["cluster_name"]

	cluster, resp, err := conn.Clusters.Get(context.Background(), projectID, clusterName)
	if err != nil {
		if resp != nil && resp.StatusCode == http.StatusNotFound {

			return nil
		}
		return fmt.Errorf(errorRead, clusterName, err)
	}

	if err := d.Set("cluster_id", cluster.ID); err != nil {
		return fmt.Errorf(errorRead, clusterName, err)
	}
	if err := d.Set("auto_scaling_disk_gb_enabled", cluster.AutoScaling.DiskGBEnabled); err != nil {
		return fmt.Errorf(errorRead, clusterName, err)
	}
	if err := d.Set("backup_enabled", cluster.BackupEnabled); err != nil {
		return fmt.Errorf(errorRead, clusterName, err)
	}
	if err := d.Set("provider_backup_enabled", cluster.ProviderBackupEnabled); err != nil {
		return fmt.Errorf(errorRead, clusterName, err)
	}
	if err := d.Set("cluster_type", cluster.ClusterType); err != nil {
		return fmt.Errorf(errorRead, clusterName, err)
	}
	if err := d.Set("disk_size_gb", cluster.DiskSizeGB); err != nil {
		return fmt.Errorf(errorRead, clusterName, err)
	}
	if err := d.Set("encryption_at_rest_provider", cluster.EncryptionAtRestProvider); err != nil {
		return fmt.Errorf(errorRead, clusterName, err)
	}
	if err := d.Set("mongo_db_major_version", cluster.MongoDBVersion); err != nil {
		return fmt.Errorf(errorRead, clusterName, err)
	}

	//Avoid Global Cluster issues. (NumShards is not present in Global Clusters)
	if cluster.NumShards != nil {
		if err := d.Set("num_shards", cluster.NumShards); err != nil {
			return fmt.Errorf(errorRead, clusterName, err)
		}
	}

	if err := d.Set("mongo_db_version", cluster.MongoDBVersion); err != nil {
		return fmt.Errorf(errorRead, clusterName, err)
	}
	if err := d.Set("mongo_uri", cluster.MongoURI); err != nil {
		return fmt.Errorf(errorRead, clusterName, err)
	}
	if err := d.Set("mongo_uri_updated", cluster.MongoURIUpdated); err != nil {
		return fmt.Errorf(errorRead, clusterName, err)
	}
	if err := d.Set("mongo_uri_with_options", cluster.MongoURIWithOptions); err != nil {
		return fmt.Errorf(errorRead, clusterName, err)
	}
	if err := d.Set("paused", cluster.Paused); err != nil {
		return fmt.Errorf(errorRead, clusterName, err)
	}
	if err := d.Set("srv_address", cluster.SrvAddress); err != nil {
		return fmt.Errorf(errorRead, clusterName, err)
	}
	if err := d.Set("state_name", cluster.StateName); err != nil {
		return fmt.Errorf(errorRead, clusterName, err)
	}
	if err := d.Set("bi_connector", flattenBiConnector(cluster.BiConnector)); err != nil {
		return fmt.Errorf(errorRead, clusterName, err)
	}
	if cluster.ProviderSettings != nil {
		flattenProviderSettings(d, *cluster.ProviderSettings)
	}
	if err := d.Set("replication_specs", flattenReplicationSpecs(cluster.ReplicationSpecs)); err != nil {
		return fmt.Errorf(errorRead, clusterName, err)
	}
	if err := d.Set("replication_factor", cluster.ReplicationFactor); err != nil {
		return fmt.Errorf(errorRead, clusterName, err)
	}

	return nil
}

func resourceMongoDBAtlasClusterUpdate(d *schema.ResourceData, meta interface{}) error {
	//Get client connection.
	conn := meta.(*matlas.Client)
	ids := decodeStateID(d.Id())
	projectID := ids["project_id"]
	clusterName := ids["cluster_name"]

	cluster := new(matlas.Cluster)

	if d.HasChange("bi_connector") {
		cluster.BiConnector, _ = expandBiConnector(d)
	}

	providerSettings := matlas.ProviderSettings{}

	// If at least one of the provider settings argument has changed, expand all provider settings
	if d.HasChange("provider_disk_iops") || d.HasChange("provider_encrypt_ebs_volume") ||
		d.HasChange("backing_provider_name") || d.HasChange("provider_disk_type_name") ||
		d.HasChange("provider_instance_size_name") || d.HasChange("provider_instance_size_name") ||
		d.HasChange("provider_instance_size_name") || d.HasChange("provider_name") ||
		d.HasChange("provider_region_name") || d.HasChange("provider_volume_type") {
		providerSettings = expandProviderSetting(d)
	}

	//Check if Provider setting was changed.
	if !reflect.DeepEqual(providerSettings, matlas.ProviderSettings{}) {
		cluster.ProviderSettings = &providerSettings
	}

	if d.HasChange("replication_specs") {
		replicationSpecs, err := expandReplicationSpecs(d)
		if err != nil {
			return fmt.Errorf(errorUpdate, clusterName, err)
		}
		cluster.ReplicationSpecs = replicationSpecs
	}

	if d.HasChange("auto_scaling_disk_gb_enabled") {
		cluster.AutoScaling.DiskGBEnabled = pointy.Bool(d.Get("auto_scaling_disk_gb_enabled").(bool))
	}
	if d.HasChange("encryption_at_rest_provider") {
		cluster.EncryptionAtRestProvider = d.Get("encryption_at_rest_provider").(string)
	}
	if d.HasChange("mongo_db_major_version") {
		cluster.MongoDBMajorVersion = d.Get("mongo_db_major_version").(string)
	}
	if d.HasChange("cluster_type") {
		cluster.ClusterType = d.Get("cluster_type").(string)
	}
	if d.HasChange("backup_enabled") {
		cluster.BackupEnabled = pointy.Bool(d.Get("backup_enabled").(bool))
	}
	if d.HasChange("disk_size_gb") {
		cluster.DiskSizeGB = pointy.Float64(d.Get("disk_size_gb").(float64))
	}
	if d.HasChange("provider_backup_enabled") {
		cluster.ProviderBackupEnabled = pointy.Bool(d.Get("provider_backup_enabled").(bool))
	}
	if d.HasChange("replication_factor") {
		cluster.ReplicationFactor = pointy.Int64(cast.ToInt64(d.Get("replication_factor")))
	}
	if d.HasChange("num_shards") {
		cluster.NumShards = pointy.Int64(cast.ToInt64(d.Get("num_shards")))
	}

	// Has changes
	if !reflect.DeepEqual(cluster, matlas.Cluster{}) {
		_, _, err := conn.Clusters.Update(context.Background(), projectID, clusterName, cluster)
		if err != nil {
			return fmt.Errorf(errorUpdate, clusterName, err)
		}
	}

	stateConf := &resource.StateChangeConf{
		Pending:    []string{"CREATING", "UPDATING", "REPAIRING"},
		Target:     []string{"IDLE"},
		Refresh:    resourceClusterRefreshFunc(clusterName, projectID, conn),
		Timeout:    d.Timeout(schema.TimeoutCreate),
		MinTimeout: 30 * time.Second,
		Delay:      1 * time.Minute,
	}

	// Wait, catching any errors
	_, err := stateConf.WaitForState()
	if err != nil {
		return fmt.Errorf(errorCreate, err)
	}

	return resourceMongoDBAtlasClusterRead(d, meta)
}

func resourceMongoDBAtlasClusterDelete(d *schema.ResourceData, meta interface{}) error {
	//Get client connection.
	conn := meta.(*matlas.Client)
	ids := decodeStateID(d.Id())
	projectID := ids["project_id"]
	clusterName := ids["cluster_name"]

	_, err := conn.Clusters.Delete(context.Background(), projectID, clusterName)

	if err != nil {
		return fmt.Errorf(errorDelete, clusterName, err)
	}

	log.Println("[INFO] Waiting for MongoDB Cluster to be destroyed")

	stateConf := &resource.StateChangeConf{
		Pending:    []string{"IDLE", "CREATING", "UPDATING", "REPAIRING", "DELETING"},
		Target:     []string{"DELETED"},
		Refresh:    resourceClusterRefreshFunc(clusterName, projectID, conn),
		Timeout:    1 * time.Hour,
		MinTimeout: 30 * time.Second,
		Delay:      1 * time.Minute, // Wait 30 secs before starting
	}

	// Wait, catching any errors
	_, err = stateConf.WaitForState()
	if err != nil {
		return fmt.Errorf(errorDelete, clusterName, err)
	}
	return nil
}

func resourceMongoDBAtlasClusterImportState(d *schema.ResourceData, meta interface{}) ([]*schema.ResourceData, error) {
	conn := meta.(*matlas.Client)

	parts := strings.SplitN(d.Id(), "-", 2)
	if len(parts) != 2 {
		return nil, errors.New("import format error: to import a cluster, use the format {project_id}-{name}")
	}

	projectID := parts[0]
	name := parts[1]

	u, _, err := conn.Clusters.Get(context.Background(), projectID, name)
	if err != nil {
		return nil, fmt.Errorf("couldn't import cluster %s in project %s, error: %s", name, projectID, err)
	}

	d.SetId(encodeStateID(map[string]string{
		"cluster_id":   u.ID,
		"project_id":   projectID,
		"cluster_name": u.Name,
	}))

	if err := d.Set("project_id", u.GroupID); err != nil {
		log.Printf("[WARN] Error setting project_id for (%s): %s", d.Id(), err)
	}
	if err := d.Set("name", u.Name); err != nil {
		log.Printf("[WARN] Error setting name for (%s): %s", d.Id(), err)
	}

	return []*schema.ResourceData{d}, nil
}

func expandBiConnector(d *schema.ResourceData) (matlas.BiConnector, error) {
	var biConnector matlas.BiConnector

	if v, ok := d.GetOk("bi_connector"); ok {
		biConnMap := v.(map[string]interface{})

		enabled := cast.ToBool(biConnMap["enabled"])

		biConnector = matlas.BiConnector{
			Enabled:        &enabled,
			ReadPreference: cast.ToString(biConnMap["read_preference"]),
		}
	}
	return biConnector, nil
}

func flattenBiConnector(biConnector matlas.BiConnector) map[string]interface{} {
	biConnectorMap := make(map[string]interface{})

	if biConnector.Enabled != nil {
		biConnectorMap["enabled"] = strconv.FormatBool(*biConnector.Enabled)
	}

	if biConnector.ReadPreference != "" {
		biConnectorMap["read_preference"] = biConnector.ReadPreference
	}

	return biConnectorMap
}

func expandProviderSetting(d *schema.ResourceData) matlas.ProviderSettings {
	diskIOPS := cast.ToInt64(d.Get("provider_disk_iops"))
	encryptEBSVolume := cast.ToBool(d.Get("provider_encrypt_ebs_volume"))

	providerSettings := matlas.ProviderSettings{
		DiskIOPS:            &diskIOPS,
		EncryptEBSVolume:    &encryptEBSVolume,
		BackingProviderName: cast.ToString(d.Get("backing_provider_name")),
		DiskTypeName:        cast.ToString(d.Get("provider_disk_type_name")),
		InstanceSizeName:    cast.ToString(d.Get("provider_instance_size_name")),
		ProviderName:        cast.ToString(d.Get("provider_name")),
		RegionName:          cast.ToString(d.Get("provider_region_name")),
		VolumeType:          cast.ToString(d.Get("provider_volume_type")),
	}

	return providerSettings
}

func flattenProviderSettings(d *schema.ResourceData, settings matlas.ProviderSettings) {
	if err := d.Set("backing_provider_name", settings.BackingProviderName); err != nil {
		log.Printf("[WARN] error setting cluster `backing_provider_name`: %s", err)
	}

	if err := d.Set("provider_disk_iops", settings.DiskIOPS); err != nil {
		log.Printf("[WARN] error setting cluster `disk_iops`: %s", err)
	}

	if err := d.Set("provider_disk_type_name", settings.DiskTypeName); err != nil {
		log.Printf("[WARN] error setting cluster `disk_type_name`: %s", err)
	}

	if err := d.Set("provider_encrypt_ebs_volume", settings.EncryptEBSVolume); err != nil {
		log.Printf("[WARN] error setting cluster `encrypt_ebs_volume`: %s", err)
	}

	if err := d.Set("provider_instance_size_name", settings.InstanceSizeName); err != nil {
		log.Printf("[WARN] error setting cluster `instance_size_name`: %s", err)
	}

	if err := d.Set("provider_name", settings.ProviderName); err != nil {
		log.Printf("[WARN] error setting cluster `provider_name`: %s", err)
	}

	if err := d.Set("provider_region_name", settings.RegionName); err != nil {
		log.Printf("[WARN] error setting cluster `region_name`: %s", err)
	}

	if err := d.Set("provider_volume_type", settings.VolumeType); err != nil {
		log.Printf("[WARN] error setting cluster `volume_type`: %s", err)
	}
}

func expandReplicationSpecs(d *schema.ResourceData) ([]matlas.ReplicationSpec, error) {
	rSpecs := make([]matlas.ReplicationSpec, 0)

	if v, ok := d.GetOk("replication_specs"); ok {
		for _, s := range v.([]interface{}) {
			spec := s.(map[string]interface{})

			regionsConfig, err := expandRegionsConfig(spec["regions_config"].(*schema.Set).List())
			if err != nil {
				return rSpecs, err
			}

			rSpec := matlas.ReplicationSpec{
				ID:            cast.ToString(spec["id"]),
				NumShards:     pointy.Int64(cast.ToInt64(spec["num_shards"])),
				ZoneName:      cast.ToString(spec["zone_name"]),
				RegionsConfig: regionsConfig,
			}
			rSpecs = append(rSpecs, rSpec)
		}
	}

	return rSpecs, nil
}

func flattenReplicationSpecs(rSpecs []matlas.ReplicationSpec) []map[string]interface{} {
	specs := make([]map[string]interface{}, 0)
	for _, rSpec := range rSpecs {
		spec := map[string]interface{}{
			"id":             rSpec.ID,
			"num_shards":     rSpec.NumShards,
			"zone_name":      rSpec.ZoneName,
			"regions_config": flattenRegionsConfig(rSpec.RegionsConfig),
		}
		specs = append(specs, spec)
	}
	return specs
}

func expandRegionsConfig(regions []interface{}) (map[string]matlas.RegionsConfig, error) {
	regionsConfig := make(map[string]matlas.RegionsConfig)
	for _, r := range regions {
		region := r.(map[string]interface{})
		r, err := cast.ToStringE(region["region_name"])
		if err != nil {
			return regionsConfig, err
		}

		regionsConfig[r] = matlas.RegionsConfig{
			AnalyticsNodes: pointy.Int64(cast.ToInt64(region["analytics_nodes"])),
			ElectableNodes: pointy.Int64(cast.ToInt64(region["electable_nodes"])),
			Priority:       pointy.Int64(cast.ToInt64(region["priority"])),
			ReadOnlyNodes:  pointy.Int64(cast.ToInt64(region["read_only_nodes"])),
		}
	}
	return regionsConfig, nil
}

func flattenRegionsConfig(regionsConfig map[string]matlas.RegionsConfig) []map[string]interface{} {
	regions := make([]map[string]interface{}, 0)

	for regionName, regionConfig := range regionsConfig {
		region := map[string]interface{}{
			"region_name":     regionName,
			"priority":        regionConfig.Priority,
			"analytics_nodes": regionConfig.AnalyticsNodes,
			"electable_nodes": regionConfig.ElectableNodes,
			"read_only_nodes": regionConfig.ReadOnlyNodes,
		}
		regions = append(regions, region)
	}
	return regions
}

func resourceClusterRefreshFunc(name, projectID string, client *matlas.Client) resource.StateRefreshFunc {
	return func() (interface{}, string, error) {
		c, resp, err := client.Clusters.Get(context.Background(), projectID, name)

		if err != nil && strings.Contains(err.Error(), "reset by peer") {
			return nil, "REPEATING", nil
		}

		if err != nil && c == nil && resp == nil {
			log.Printf("Error reading MongoDB cluster: %s: %s", name, err)
			return nil, "", err
		} else if err != nil {
			if resp.StatusCode == 404 {
				return 42, "DELETED", nil
			}
			log.Printf("Error reading MongoDB Cluster %s: %s", name, err)
			return nil, "", err
		}

		if c.StateName != "" {
			log.Printf("[DEBUG] status for MongoDB cluster: %s: %s", name, c.StateName)
		}

		return c, c.StateName, nil
	}
}
