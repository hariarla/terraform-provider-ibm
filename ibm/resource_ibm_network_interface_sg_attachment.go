package ibm

import (
	"fmt"
	"log"
	"strconv"
	"strings"
	"time"

	"github.com/hashicorp/terraform/helper/resource"
	"github.com/hashicorp/terraform/helper/schema"
	"github.com/softlayer/softlayer-go/services"
	slsession "github.com/softlayer/softlayer-go/session"
	"github.com/softlayer/softlayer-go/sl"
)

func resourceIBMNetworkInterfaceSGAttachment() *schema.Resource {
	return &schema.Resource{
		Create: resourceIBMNetworkInterfaceSGAttachmentCreate,
		Read:   resourceIBMNetworkInterfaceSGAttachmentRead,
		Delete: resourceIBMNetworkInterfaceSGAttachmentDelete,
		Exists: resourceIBMNetworkInterfaceSGAttachmentExists,
		Schema: map[string]*schema.Schema{
			"security_group_id": {
				Type:     schema.TypeInt,
				Required: true,
				ForceNew: true,
			},
			"network_interface_id": {
				Type:     schema.TypeInt,
				Required: true,
				ForceNew: true,
			},
			"soft_reboot": {
				Type:     schema.TypeBool,
				Optional: true,
				Default:  true,
				ForceNew: true,
			},
		},
	}
}

func resourceIBMNetworkInterfaceSGAttachmentCreate(d *schema.ResourceData, meta interface{}) error {
	mk := "network_interface_sg_attachment_" + strconv.Itoa(d.Get("network_interface_id").(int))
	ibmMutexKV.Lock(mk)
	defer ibmMutexKV.Unlock(mk)

	sess := meta.(ClientSession).SoftLayerSession()
	service := services.GetNetworkSecurityGroupService(sess)

	sgID := d.Get("security_group_id").(int)
	interfaceID := d.Get("network_interface_id").(int)

	_, err := service.Id(sgID).AttachNetworkComponents([]int{interfaceID})
	if err != nil {
		return err
	}
	d.SetId(fmt.Sprintf("%d_%d", sgID, interfaceID))

	// If user has not explicity disabled soft reboot
	if ok := d.Get("soft_reboot").(bool); ok {
		//Check if a soft reboot is required and perform it
		ncs := services.GetVirtualGuestNetworkComponentService(sess)
		ready, err := ncs.Id(interfaceID).SecurityGroupsReady()
		if err != nil {
			return err
		}
		if !ready {
			log.Println("Soft reboot the VSI whose network component is", interfaceID)
		}
		guest, err := ncs.Id(interfaceID).GetGuest()
		if err != nil {
			return fmt.Errorf("Couldn't retrieve the virtual guest on interface %d", interfaceID)
		}
		guestService := services.GetVirtualGuestService(sess)
		ok, err := guestService.Id(*guest.Id).RebootSoft()
		if err != nil {
			return err
		}
		if !ok {
			return fmt.Errorf("Couldn't reboot the VSI %d", *guest.Id)
		}
		//Wait for security group to be ready again after reboot
		stateConf := &resource.StateChangeConf{
			Target:  []string{"true"},
			Pending: []string{"false"},
			Timeout: 5 * time.Minute,
			Refresh: securityGroupReadyRefreshStateFunc(sess, interfaceID),
		}
		_, err = stateConf.WaitForState()
		if err != nil {
			return err
		}
	}

	return resourceIBMNetworkInterfaceSGAttachmentRead(d, meta)
}

func resourceIBMNetworkInterfaceSGAttachmentRead(d *schema.ResourceData, meta interface{}) error {
	sess := meta.(ClientSession).SoftLayerSession()
	service := services.GetNetworkSecurityGroupService(sess)
	sgID, interfaceID, err := decomposeNetworkSGAttachmentID(d.Id())
	if err != nil {
		return err
	}
	bindings, err := service.Id(sgID).GetNetworkComponentBindings()
	if err != nil {
		return err
	}
	for _, b := range bindings {
		if *b.NetworkComponentId == interfaceID {
			return nil
		}
	}
	return fmt.Errorf("No association found between security group %d and network interface %d", sgID, interfaceID)
}

func resourceIBMNetworkInterfaceSGAttachmentDelete(d *schema.ResourceData, meta interface{}) error {
	mk := "network_interface_sg_attachment_" + strconv.Itoa(d.Get("network_interface_id").(int))
	ibmMutexKV.Lock(mk)
	defer ibmMutexKV.Unlock(mk)
	sess := meta.(ClientSession).SoftLayerSession()
	service := services.GetNetworkSecurityGroupService(sess)
	sgID, interfaceID, err := decomposeNetworkSGAttachmentID(d.Id())
	if err != nil {
		return err
	}
	_, err = service.Id(sgID).DetachNetworkComponents([]int{interfaceID})
	if err != nil {
		return fmt.Errorf("Error detaching network components from Security Group: %s", err)
	}
	d.SetId("")
	return nil
}

func resourceIBMNetworkInterfaceSGAttachmentExists(d *schema.ResourceData, meta interface{}) (bool, error) {
	sess := meta.(ClientSession).SoftLayerSession()
	service := services.GetNetworkSecurityGroupService(sess)

	sgID, interfaceID, err := decomposeNetworkSGAttachmentID(d.Id())
	if err != nil {
		return false, err
	}

	bindings, err := service.Id(sgID).GetNetworkComponentBindings()
	if err != nil {
		if apiErr, ok := err.(sl.Error); ok {
			if apiErr.StatusCode == 404 {
				return false, nil
			}
		}
		return false, fmt.Errorf("Error communicating with the API: %s", err)
	}
	for _, b := range bindings {
		if *b.NetworkComponentId == interfaceID {
			return true, nil
		}
	}
	return false, fmt.Errorf("No association found between security group %d and network interface %d", sgID, interfaceID)
}

func decomposeNetworkSGAttachmentID(attachmentID string) (sgID, interfaceID int, err error) {
	ids := strings.Split(attachmentID, "_")
	if len(ids) != 2 {
		return -1, -1, fmt.Errorf("The ibm_network_interface_sg_attachment id must be of the form <sg_id>_<network_interface_id> but it is %s", attachmentID)
	}
	sgID, err = strconv.Atoi(ids[0])
	if err != nil {
		return -1, -1, fmt.Errorf("Not a valid security group ID, must be an integer: %s", err)
	}

	interfaceID, err = strconv.Atoi(ids[1])
	if err != nil {
		return -1, -1, fmt.Errorf("Not a valid network interface ID, must be an integer: %s", err)
	}
	return
}

func securityGroupReadyRefreshStateFunc(sess *slsession.Session, ifcID int) resource.StateRefreshFunc {
	return func() (interface{}, string, error) {
		ncs := services.GetVirtualGuestNetworkComponentService(sess)
		ready, err := ncs.Id(ifcID).SecurityGroupsReady()
		if err != nil {
			return ready, "false", err
		}
		log.Printf("SecurityGroupReady status is %t", ready)
		return ready, strconv.FormatBool(ready), nil
	}
}
