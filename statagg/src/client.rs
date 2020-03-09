mod gatewaypb {
    tonic::include_proto!("gateway");
}

use gatewaypb::gateway_client;
use std::error::Error;
use tonic::transport::channel::Channel;

pub type GatewayClient = gateway_client::GatewayClient<Channel>;
pub use gatewaypb::{EmptyRequest, Stat, StatsResponse};

pub async fn get_clients(clusters: i32) -> Result<Vec<GatewayClient>, Box<dyn Error>> {
    let mut conns: Vec<GatewayClient> = Vec::new();

    for i in 0..clusters {
        let conn = GatewayClient::connect(format!("gateway-{}.tatsu.svc.cluster.local", i)).await?;
        conns.push(conn);
    }

    Ok(conns)
}
