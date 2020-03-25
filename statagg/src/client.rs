mod gatewaypb {
    tonic::include_proto!("gateway");
}

use gatewaypb::gateway_client;
use std::error::Error;
use tonic::transport::channel::Channel;

pub type GatewayClient = gateway_client::GatewayClient<Channel>;
pub use gatewaypb::{EmptyRequest, Stat, StatsResponse};

pub async fn get_clients(clusters: i32) -> Result<Vec<GatewayClient>, Box<dyn Error>> {
    let mut conns: Vec<GatewayClient> = Vec::with_capacity(clusters as usize);

    for i in 0..clusters {
        let url = format!("http://gateway-{}.gateway.tatsu.svc.cluster.local", i);
        println!("{}", url);
        let conn = GatewayClient::connect(url).await?;
        conns.push(conn);
    }

    Ok(conns)
}
