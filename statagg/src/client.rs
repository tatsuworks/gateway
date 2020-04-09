mod gatewaypb {
    tonic::include_proto!("gateway");
}

use gatewaypb::gateway_client;
use std::env;
use std::error::Error;
use tonic::transport::channel::Channel;

pub type GatewayClient = gateway_client::GatewayClient<Channel>;
pub use gatewaypb::{EmptyRequest, Stat, StatsResponse};

pub async fn get_clients(clusters: i32) -> Result<Vec<GatewayClient>, Box<dyn Error>> {
    let ns = env::var("NAMESPACE").unwrap_or("tatsu".to_string());
    let mut conns: Vec<GatewayClient> = Vec::with_capacity(clusters as usize);

    for i in 0..clusters {
        let url = format!("http://gateway-{}.{}.tatsu.svc.cluster.local", i, ns);
        println!("{}", url);
        let conn = GatewayClient::connect(url).await?;
        conns.push(conn);
    }

    Ok(conns)
}
