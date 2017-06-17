package openstack


import (
	"fmt"
	"time"
	"github.com/rackspace/gophercloud"
	"github.com/rackspace/gophercloud/openstack"
	"github.com/rackspace/gophercloud/openstack/compute/v2/servers"
	"github.com/rackspace/gophercloud/pagination"
	"github.com/rackspace/gophercloud/openstack/compute/v2/images"

	"weibo.com/opendcp/jupiter/provider"
	"sync"
	"github.com/rackspace/gophercloud/openstack/compute/v2/extensions/startstop"
	"weibo.com/opendcp/jupiter/models"
	"github.com/rackspace/gophercloud/openstack/compute/v2/flavors"
	"github.com/rackspace/gophercloud/openstack/networking/v2/networks"
)

//1.由于接口完全是阿里云的接口，已经实现的函数无法实现相应功能
//2.无法实现功能的方法如何处理

type openstackProvider struct {
	client *gophercloud.ServiceClient
	lock   sync.Mutex
}

func init(){

	provider.RegisterProviderDriver("openstack", new)
}


var instanceTypesInOpenStack map[string]string
var instanceTypesList []string
var networksInOpenStack map[string]string
var networksList []string

//列出所有server
//openstack不需要提供pageNumber和pageSize,该如何处理
//返回的示例中包含所有信息，之后根据需要进行适当的删减
//要求：搞清楚前端调用时到底需要哪些参数，以什么顺序排列
func (driver openstackProvider) List(regionId string, pageNumber int, pageSize int) (*models.ListInstancesResponse, error) {
	opts1 := servers.ListOpts{}
	pager := servers.List(driver.client, opts1)
	var listInstancesResp models.ListInstancesResponse
	err := pager.EachPage(func(page pagination.Page) (bool, error) {
		serverList, _ := servers.ExtractServers(page)
		for _, instanceOP := range serverList {
			var instance models.InstanceAllIn
			instance.InstanceId = instanceOP.ID
			instance.TenantID = instanceOP.TenantID
			instance.UserID = instanceOP.UserID
			instance.Name = instanceOP.Name
			instance.Updated = instanceOP.Updated
			instance.Created = instanceOP.Created
			instance.HostID = instanceOP.HostID
			instance.Status = instanceOP.Status
			instance.Progress = instanceOP.Progress
			instance.AccessIPv4 = instanceOP.AccessIPv4
			instance.AccessIPv6 = instanceOP.AccessIPv6
			//instance.Image = instanceOP.Image
			//instance.Flavor = instanceOP.Flavor
			//instance.Addresses = instanceOP.Addresses
			//instance.Metadata = instanceOP.Metadata
			//instance.Links = instanceOP.Links
			//instance.KeyName = instanceOP.KeyName
			//instance.AdminPass = instanceOP.AdminPass
			//instance.SecurityGroups = instanceOP.SecurityGroups
			listInstancesResp.Reservations = append(listInstancesResp.Reservations, instance)
		}
		return  true, nil
	})



	return &listInstancesResp, err
}


//将instanceType对应OpenStack中的flavor
//openstack中的获取InstanceType方法待做，需要与创建机型模板那边联动
func (driver openstackProvider) ListInstanceTypes() ([]string, error){

	
	if instanceTypesInOpenStack != nil{
		return instanceTypesList, nil
	}
	opts := flavors.ListOpts{}
	pager := flavors.ListDetail(driver.client, opts)
	err := pager.EachPage(func(page pagination.Page) (bool, error) {

		flavorList, err := flavors.ExtractFlavors(page)
		for _, flavor := range flavorList {
			instanceTypesList = append(instanceTypesList, flavor.Name)
			instanceTypesInOpenStack[flavor.Name] = flavor.ID
		}
		return true, err
	})


	return instanceTypesList, err
}

func (driver openstackProvider) ListSecurityGroup(regionId string, vpcId string) (*models.SecurityGroupsResp, error){
	return nil, nil
}

func (driver openstackProvider) ListAvailabilityZones(regionId string) (*models.AvailabilityZonesResp, error){
	return nil, nil
}

func (driver openstackProvider) ListRegions() (*models.RegionsResp, error){
	return nil, nil
}

func (driver openstackProvider) ListVpcs(regionId string, pageNumber int, pageSize int) (*models.VpcsResp, error){
	return nil, nil
}

func (driver openstackProvider) ListSubnets(zoneId string, vpcId string) (*models.SubnetsResp, error){
	return nil, nil
}

func (driver openstackProvider) GetInstanceType(key string) string{
	return ""
}

func (driver openstackProvider) ListDiskCategory() []string{
	return nil
}

func (driver openstackProvider) ListInternetChargeType() []string{
	return nil
}

func (driver openstackProvider) AllocatePublicIpAddress(instanceId string) (string, error){
	return "", nil
}


//创建实例代码待做
func (driver openstackProvider) Create(cluster *models.Cluster, number int) ([]string, []error) {

	createdInstances := make(chan string, number)
	createdError := make(chan error, number)
	for i := 0; i < number; i++ {
		go func(i int) {
			fmt.Println("first create")
			result, err := servers.Create(driver.client, servers.CreateOpts{
				Name:      cluster.Name ,
				ImageRef:  cluster.ImageId,
				FlavorRef: cluster.FlavorId,
				AvailabilityZone: cluster.Zone.ZoneName,
				Networks: []servers.Network{{UUID: cluster.Network.VpcId}},
			}).Extract()
			if err != nil {
				for i := 0; i < 3; i++ {
					fmt.Println("try to create instance")
					result, err := servers.Create(driver.client, servers.CreateOpts{
						Name:      cluster.Name ,
						ImageRef:  cluster.ImageId,
						FlavorRef: cluster.FlavorId,
						AvailabilityZone: cluster.Zone.ZoneName,
						Networks: []servers.Network{{UUID: cluster.Network.VpcId}},
					}).Extract()
					if err == nil {
						fmt.Println("create success")
						createdInstances <- result.ID
						return
					}
				}
				createdError <- err
				return
			}
			fmt.Println("create success")
			createdInstances <- result.ID
		}(i)
	}
	instanceIds := make([]string, 0)
	errs := make([]error, 0)
	for i := 0; i < number; i++ {
		select {
		case instanceId := <-createdInstances:
			instanceIds = append(instanceIds, instanceId)
		case err := <-createdError:
			errs = append(errs, err)
		}
	}

	//待解决问题：不管产不产生error，传回的errs变量都不为nil,在service/instance的方法里都会返回，故在此返回nil，日后找到原因后再改为errs
	return instanceIds, errs
}



func (driver openstackProvider) GetInstance(instanceId string) (*models.Instance, error) {

	server, err := servers.Get(driver.client, instanceId).Extract()
	if err != nil {
		return nil, err
	}
	var instance models.Instance

	instance.InstanceId = server.ID
	instance.Provider = "openstack"
	instance.CreateTime, _ = time.ParseInLocation("2006-01-02 15:04:05", server.Created, time.Local)
	tmp := server.Image["id"]
	instance.ImageId = tmp.(string)
	//InstanceType
	//VpcId
	//subnetId
	//SecurityGroupsId
	//私有Ip和公有Ip替换为IPV4和IPV6
	instance.AccessIPv4 = server.AccessIPv4
	instance.AccessIPv6 = server.AccessIPv6
	instance.Name = server.Name
	instance.TenantID = server.TenantID
	instance.UserID = server.UserID
	return &instance, err
}

//列出镜像列表
//这里使用的镜像是阿里云的镜像，之后根据情况添加openstack镜像的相关参数
func (driver openstackProvider) ListImages(regionId string, snapshotId string, pageSize int, pageNumber int) (*models.ImagesResp, error) {


	opts1 := images.ListOpts{}
	pager := images.ListDetail(driver.client, opts1)
	var imageResp models.ImagesResp
	timages := make([]models.Image, 0)
	pager.EachPage(func(page pagination.Page) (bool, error) {
		imageList, err := images.ExtractImages(page)
		for _, imageOp := range imageList {
			image := models.Image{

				//Architecture: imageOp.
				CreationDate: imageOp.Created,
				//Description: imageOp.
				ImageId: imageOp.ID,
				Name: imageOp.Name,
				//OwnerId: imageOp.
				//ProductCodes
				State: imageOp.Status,

			}
			timages = append(timages, image)
		}

		return true, err
	})
	imageResp.Images = timages
	return &imageResp, nil
}

func (driver openstackProvider) Start(instanceId string) (bool, error) {


	err := startstop.Start(driver.client, instanceId).ExtractErr()

	return true, err
}

func (driver openstackProvider) Stop(instanceId string) (bool, error) {


	err1 := startstop.Stop(driver.client, instanceId).ExtractErr()

	return true, err1
}

//删除实例
func (driver openstackProvider) Delete(instanceId string) (time.Time, error) {


	server, err := servers.Get(driver.client, instanceId).Extract()

	if err != nil {
		return time.Now(), err
	}
	if server.Status != "Stopped" {
		startstop.Stop(driver.client, instanceId).ExtractErr()

		waitForSpecific(func() bool {
			server, err := servers.Get(driver.client, instanceId).Extract()
			if err != nil {
				return false
			}
			return server.Status == "Stopped"
		}, 10, 6*time.Second)
	}
	time.Sleep(5 * time.Second)
	result := servers.Delete(driver.client, instanceId)

	if result.Err != nil {
		return time.Now(), err
	}
	return time.Now(), nil
}

func (driver openstackProvider) WaitForInstanceToStop(instanceId string) bool {
	st, err := driver.GetState(instanceId)
	if err != nil {
		return false
	}
	return st == models.Stopped
}

func (driver openstackProvider) WaitToStartInstance(instanceId string) bool {
	st, err := driver.GetState(instanceId)
	if err != nil {
		return false
	}
	return st == models.Running
}

func (driver openstackProvider) GetState(instanceId string) (models.InstanceState, error) {

	server, err := servers.Get(driver.client, instanceId).Extract()
	if err != nil {
		return models.StateError, err
	}
	switch server.Status {
	case "Running":
		return models.Running, nil
	case "Starting":
		return models.Starting, nil
	case "Stopped":
		return models.Stopped, nil
	case "Stopping":
		return models.Stopping, nil
	default:
		return models.None, nil
	}
}

func waitForSpecific(f func() bool, maxAttempts int, waitInterval time.Duration) error {
	for i := 0; i < maxAttempts; i++ {
		if f() {
			return nil
		}
		time.Sleep(waitInterval)
	}
	return fmt.Errorf("Maximum number of retries (%d) exceeded", maxAttempts)
}

func (driver openstackProvider) ListNetworks() ([]string, error){
	opts := gophercloud.AuthOptions{
		IdentityEndpoint: "http://10.39.59.27:5000/v3",
		Username: "admin",
		Password: "ZYGL32NDG7JS8IGC",
		DomainName: "default",
	}

	provider, err := openstack.AuthenticatedClient(opts)

	client, err := openstack.NewNetworkV2(provider, gophercloud.EndpointOpts{
		Name:   "neutron",
		Region: "RegionOne",
	})
	opts1 := networks.ListOpts{}
	// Retrieve a pager (i.e. a paginated collection)
	pager := networks.List(client, opts1)

	netList := make([]string,0)

	err = pager.EachPage(func(page pagination.Page) (bool, error) {
		networkList, err := networks.ExtractNetworks(page)
		for _, network := range networkList {
			// "n" will be a networks.Network
			netList = append(netList, network.Name)
		}

		return true, err
	})
	return netList, err
}

func new() (provider.ProviderDriver, error){

	return newProvider()
}
func newProvider() (provider.ProviderDriver, error){
	opts := gophercloud.AuthOptions{
		IdentityEndpoint: "http://10.39.59.27:5000/v3",
		Username: "admin",
		Password: "ZYGL32NDG7JS8IGC",
		DomainName: "default",
	}

	provider, err := openstack.AuthenticatedClient(opts)

	client, err :=
		openstack.NewComputeV2(provider, gophercloud.EndpointOpts{
			Region: "RegionOne",
		})

	ret := openstackProvider{
		client: client,
	}
	return ret, err
}


















