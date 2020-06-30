exports.handler = async (event, context, callback) => {
    const request = event.Records[0].cf.request;
  
    request.uri = "/content" + request.uri;
  
    return callback(null, request);
  };
  