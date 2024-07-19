pipeline {
    agent {
        label "swarm"
    }

    environment {
        APP_NAME = "kok"

        PROJECT_NAME = "buxiaomo"
        PROJECT_ENV = "dev"

        REPOSITORY_URL = "https://github.com/buxiaomo/kok.git"

        REGISTRY_HOST = "172.16.115.11:5000"
    }

    parameters{
        text(
            description: "helm deployment parameter.",
            name: "opts",
            defaultValue: "--set webhookUrl=http://<EXTERNAL-IP>:8080 ",
        )
        booleanParam(
            defaultValue: true,
            description: "auto deploy to env?",
            name: 'autodeploy',
        )

        choice(
            description: 'Docker image Arch?',
            choices: ['amd64', 'arm64', 'amd64,arm64'],
            name: 'arch',
        )
    }

    options {
        disableConcurrentBuilds abortPrevious: true
    }

    stages {
        stage('checkout') {
            steps {
                checkout scmGit(branches: [[name: '*/main']], extensions: [], userRemoteConfigs: [[url: "${env.REPOSITORY_URL}"]])
            }
        }

        stage('compile') {
            steps {
                sh label: 'build image', script: "nerdctl build --platform=${params.arch} --output type=image,name=${env.REGISTRY_HOST}/${env.PROJECT_NAME}/${env.APP_NAME}:${BUILD_NUMBER},push=true -f Dockerfile ."
            }
        }

        stage('deploy') {
            steps {
                script{
                    if(params.autodeploy) {
                        withKubeConfig(caCertificate: '', clusterName: 'kubernetes', contextName: 'default', credentialsId: 'kubeconfig', namespace: 'kube-system', restrictKubeConfigAccess: false, serverUrl: 'https://172.16.115.11:6443') {
                            sh "helm upgrade -i kok --set hub=${env.REGISTRY_HOST}/${env.PROJECT_NAME}/${env.APP_NAME} --set tag=${BUILD_NUMBER} ${params.opts} kok --create-namespace --namespace ${env.PROJECT_NAME}-${env.PROJECT_ENV}"
                        }
                    }
                }
            }
        }
    }
}