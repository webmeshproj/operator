terraform {
  required_providers {
    google = {
      source = "hashicorp/google"
    }
    kubernetes = {
      source = "hashicorp/kubernetes"
    }
    helm = {
      source = "hashicorp/helm"
    }
  }
}

variable "project_id" {
  type = string
}

variable "region" {
  type    = string
  default = "us-central1"
}

variable "name" {
  type    = string
  default = "webmesh"
}

variable "email" {
  type    = string
  default = ""
}

data "google_client_config" "default" {}

locals {
  name                   = var.name
  region                 = var.region
  gke_pods_name          = "${local.name}-gke-pods"
  gke_services_name      = "${local.name}-gke-services"
  private_internal_cidr  = "10.0.0.0/16"
  private_external_cidr  = "10.254.0.0/16"
  pod_cidr_range         = "10.10.0.0/16"
  svc_cidr_range         = "10.20.0.0/16"
  cluster_endpoint       = google_container_cluster.this.endpoint
  cluster_host           = "https://${local.cluster_endpoint}"
  cluster_ca_certificate = base64decode(google_container_cluster.this.master_auth.0.cluster_ca_certificate)
}

provider "google" {
  project = var.project_id
  region  = var.region
}

provider "kubernetes" {
  host                   = local.cluster_host
  cluster_ca_certificate = local.cluster_ca_certificate
  token                  = data.google_client_config.default.access_token
}

provider "helm" {
  kubernetes {
    host                   = local.cluster_host
    cluster_ca_certificate = local.cluster_ca_certificate
    token                  = data.google_client_config.default.access_token
  }
}

# VPC Network

resource "google_compute_network" "this" {
  project                  = var.project_id
  name                     = "${local.name}-network"
  mtu                      = 1460
  routing_mode             = "REGIONAL"
  enable_ula_internal_ipv6 = true
  auto_create_subnetworks  = false
}

resource "google_compute_subnetwork" "internal" {
  name    = "${local.name}-internal"
  project = var.project_id

  ip_cidr_range = local.private_internal_cidr
  region        = local.region

  stack_type       = "IPV4_IPV6"
  ipv6_access_type = "INTERNAL"

  network = google_compute_network.this.id

  secondary_ip_range {
    range_name    = local.gke_pods_name
    ip_cidr_range = local.pod_cidr_range
  }

  secondary_ip_range {
    range_name    = local.gke_services_name
    ip_cidr_range = local.svc_cidr_range
  }
}

resource "google_compute_subnetwork" "external" {
  name    = "${local.name}-external"
  project = var.project_id

  ip_cidr_range    = local.private_external_cidr
  region           = local.region
  stack_type       = "IPV4_IPV6"
  ipv6_access_type = "EXTERNAL"
  network          = google_compute_network.this.id
}

# Container Cluster

resource "google_service_account" "cluster_default" {
  account_id   = "${local.name}-cluster-default"
  project      = var.project_id
  display_name = "Default service account for ${local.name} cluster"
}

resource "google_container_cluster" "this" {
  name       = "${local.name}-cluster"
  project    = var.project_id
  location   = local.region
  network    = google_compute_network.this.name
  subnetwork = google_compute_subnetwork.internal.name

  release_channel {
    channel = "REGULAR"
  }

  remove_default_node_pool = true
  initial_node_count       = 1

  datapath_provider = "ADVANCED_DATAPATH"

  ip_allocation_policy {
    cluster_secondary_range_name  = local.gke_pods_name
    services_secondary_range_name = local.gke_services_name
    stack_type                    = "IPV4_IPV6"
  }

  enable_l4_ilb_subsetting = true

  workload_identity_config {
    workload_pool = "${var.project_id}.svc.id.goog"
  }

  logging_config {
    enable_components = []
  }

  monitoring_config {
    enable_components = []
  }
}

resource "google_container_node_pool" "this" {
  name     = "${local.name}-nodes"
  project  = var.project_id
  location = google_container_cluster.this.location
  cluster  = google_container_cluster.this.name

  initial_node_count = 1

  node_config {
    machine_type    = "e2-standard-2"
    disk_size_gb    = "60"
    disk_type       = "pd-standard"
    image_type      = "COS_CONTAINERD"
    service_account = google_service_account.cluster_default.email

    oauth_scopes = ["https://www.googleapis.com/auth/cloud-platform"]

    workload_metadata_config {
      mode = "GKE_METADATA"
    }

    gvnic {
      enabled = true
    }
  }

  network_config {
    pod_range            = local.gke_pods_name
    enable_private_nodes = false
  }

  autoscaling {
    min_node_count = 1
    max_node_count = 3
  }

  management {
    auto_repair  = false
    auto_upgrade = true
  }
}

# Cert Manager

resource "helm_release" "cert_manager" {
  name             = "cert-manager"
  repository       = "https://charts.jetstack.io"
  chart            = "cert-manager"
  namespace        = "cert-manager"
  create_namespace = true
  set {
    name  = "installCRDs"
    value = "true"
  }

  depends_on = [google_container_cluster.this]
}

# Operator workload identity

module "operator_workload_identity" {
  source = "terraform-google-modules/kubernetes-engine/google//modules/workload-identity"

  project_id = var.project_id
  name       = "operator-controller-manager"
  namespace  = "webmesh-system"
  roles = [
    "roles/compute.instanceAdmin",
  ]
  annotate_k8s_sa     = false
  use_existing_k8s_sa = true
}

output "operator_workload_identity" {
  value = module.operator_workload_identity
}

# Firewall Rules

resource "google_compute_firewall" "routers_ipv4" {
  project     = var.project_id
  name        = "allow-nodes-inbound-ipv4"
  network     = google_compute_network.this.name
  description = "Allow nodes inbound traffic"

  allow {
    protocol = "all"
  }

  direction     = "INGRESS"
  source_ranges = ["0.0.0.0/0"]
  target_tags   = ["mesh-nodes"]
}

resource "google_compute_firewall" "routers_ipv6" {
  project     = var.project_id
  name        = "allow-nodes-inbound-ipv6"
  network     = google_compute_network.this.name
  description = "Allow nodes inbound traffic"

  allow {
    protocol = "all"
  }

  direction     = "INGRESS"
  source_ranges = ["::/0"]
  target_tags   = ["mesh-nodes"]
}

resource "google_compute_firewall" "iap_tcp" {
  count       = var.email != "" ? 1 : 0
  project     = var.project_id
  name        = "${local.name}-iap-tcp"
  network     = google_compute_network.this.name
  description = "Allow access from IAP"

  allow {
    protocol = "tcp"
    ports    = ["22"]
  }

  direction     = "INGRESS"
  source_ranges = ["35.235.240.0/20"]
}

# IAP Tunnel Access

resource "google_iap_tunnel_iam_binding" "tcp_access" {
  count   = var.email != "" ? 1 : 0
  project = var.project_id
  role    = "roles/iap.tunnelResourceAccessor"
  members = [
    "user:${var.email}",
  ]
}
