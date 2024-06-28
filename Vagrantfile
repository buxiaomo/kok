# -*- mode: ruby -*-
# vi: set ft=ruby :

# $ ssh -i .ssh/id_rsa root@192.168.56.10

# $ mkdir .ssh
# $ ssh-keygen -t rsa -P "" -f ./.ssh/id_rsa

VAGRANTFILE_API_VERSION = "2"

cluster = {
  "worker01" => { :ip => "192.168.56.11", :cpus => 2, :mem => 2048 },
  "worker02" => { :ip => "192.168.56.12", :cpus => 2, :mem => 2048 },
  "master01" => { :ip => "192.168.56.10", :cpus => 2, :mem => 4096 },
}

Vagrant.configure(VAGRANTFILE_API_VERSION) do |config|
  config.env.enable
  cluster.each_with_index do |(hostname, info), index|
    config.vm.define hostname do |cfg|
      cfg.vm.provider :virtualbox do |vb, override|
        config.vm.box = "ubuntu/jammy64"
        override.vm.network :private_network, ip: "#{info[:ip]}"
        override.vm.hostname = hostname
        vb.name = hostname
        vb.cpus = info[:cpus]
        vb.memory = info[:mem]
      end
      cfg.vm.provision "shell" do |s|
        s.path = "./tests/prepare.sh"
        s.args = [ "1.30.2", "containerd", "flannel", "vagrant" ]
      end
    end
  end
end