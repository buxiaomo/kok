# kok

This project is deploy kubernetes control-plane on k8s.

## try it
```shell
helm upgrade -i kok ./kok -n kok --create-namespace

# get EXTERNAL-IP and redeploy
helm upgrade -i kok ./kok -n kok --create-namespace \
--set webhookUrl=http://<EXTERNAL-IP>:8080 
```

Now you can open the link to create the cluster
* http://<EXTERNAL-IP>:8080/console/cluster