# Building a custom fedora containerdisk for dhcpserver

This follows the scripts provided in kubevirtci repo. This was tested on the following version of libvirt on centos7

    $ virsh version
    Compiled against library: libvirt 4.5.0
    Using library: libvirt 4.5.0
    Using API: QEMU 4.5.0
    Running hypervisor: QEMU 1.5.3
    
There are three files that you need.

1. cloud-config: This contains the cloud-init script to run in the image. You should add you setup commands in here. If you wish to run some customizations manually, remove the shutdown command in the script. Once you are done with youe changes, you can either shutdown the VM or close the domain with ^].
2. image-url: URL of the image.
3. os-variant: Passed as a parameter to virt-install. You may have to update your osinfo-db if `osinfo-query os` does not contain your os variant. You can do this by downloading the latest db and importing it

```shell

    wget https://releases.pagure.org/libosinfo/osinfo-db-20221130.tar.xz
    osinfo-db-import -v osinfo-db-20221130.tar.xz

```

## Build and Push it

    ```shell

    # Build your dhcpserver image
    cd dhcpserver
    make build

    # Then upload dhcpserver/bin/manager and provide the link in the cloud-init script
    # Now you can start customizing the image

    cd dhcpserver/dhcpserver-image
    ./create-containerdisk.sh fedora
    ./publish-containerdisk.sh fedora <docker_repo>
    
    ```
